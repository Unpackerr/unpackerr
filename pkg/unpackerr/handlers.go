package unpackerr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

// Extract holds data for files being extracted.
type Extract struct {
	Syncthing   bool
	Retries     uint
	Path        string
	App         starr.App
	URL         string
	Updated     time.Time
	DeleteDelay time.Duration
	DeleteOrig  bool
	Status      ExtractStatus
	IDs         map[string]interface{}
	Resp        *xtractr.Response
}

// Shared config items for all starr apps.
type StarrConfig struct {
	Path        string        `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths       StringSlice   `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols   string        `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	DeleteOrig  bool          `json:"delete_orig" toml:"delete_orig" xml:"delete_orig" yaml:"delete_orig"`
	DeleteDelay cnfg.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Syncthing   bool          `json:"syncthing" toml:"syncthing" xml:"syncthing" yaml:"syncthing"`
	ValidSSL    bool          `json:"valid_ssl" toml:"valid_ssl" xml:"valid_ssl" yaml:"valid_ssl"`
	Timeout     cnfg.Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
}

// checkQueueChanges checks each item for state changes from the app queues.
func (u *Unpackerr) checkQueueChanges() {
	for name, data := range u.Map {
		switch {
		case data.App == FolderString:
			continue // folders are handled in folder.go.
		case !u.haveQitem(name, data.App):
			// This fires when an items becomes missing (imported) from the application queue.
			switch elapsed := time.Since(data.Updated); {
			case data.Status == WAITING:
				// A waiting item just fell out of the queue. We never extracted it. Remove it and move on.
				delete(u.Map, name)
				u.Printf("[%v] Imported: %v (not extracted, removing from history)", data.App, name)
			case data.Status > IMPORTED:
				u.Debugf("Already imported? %s", name)
			case data.Status == IMPORTED:
				u.Debugf("%v: Awaiting Delete Delay (%v remains): %v",
					data.App, data.DeleteDelay-elapsed.Round(time.Second), name)
			default:
				u.updateQueueStatus(&newStatus{Name: name, Status: IMPORTED, Resp: data.Resp}, true)
				u.Printf("[%v] Imported: %v (delete in %v)", data.App, name, data.DeleteDelay)
			}
		case data.Status == IMPORTED:
			// The item fell out of the app queue and came back. Reset it.
			u.Printf("%s: Extraction Not Imported: %s - De-queued and returned.", data.App, name)
			data.Status = EXTRACTED
		case data.Status > IMPORTED:
			// The item fell out of the app queue and came back. Reset it.
			u.Printf("%s: Extraction Restarting: %s - Deleted Item De-queued and returned.", data.App, name)
			data.Status = WAITING
			data.Updated = time.Now()
		}

		u.Debugf("%s: Status: %s (%v, elapsed: %v)", data.App, name, data.Status.Desc(),
			time.Since(data.Updated).Round(time.Second))
	}
}

// extractCompletedDownloads process each download and checks if it needs to be extracted.
// This is called from the main go routine in start.go and it only process starr apps, not folders.
func (u *Unpackerr) extractCompletedDownloads() {
	for name, item := range u.Map {
		if item.App != FolderString {
			u.extractCompletedDownload(name, item)
		}
	}
}

// extractCompletedDownload checks if a completed starr download needs to be extracted.
// This is called by extractCompletedDownloads() via the main routine in start.go.
func (u *Unpackerr) extractCompletedDownload(name string, item *Extract) {
	if time.Since(item.Updated) < u.Config.StartDelay.Duration {
		u.Printf("[%s] Waiting for Start Delay: %v (%v remains)", item.App, name,
			u.Config.StartDelay.Duration-time.Since(item.Updated).Round(time.Second))

		return
	}

	files := xtractr.FindCompressedFiles(xtractr.Filter{Path: item.Path})
	if len(files) == 0 {
		if _, err := os.Stat(item.Path); err != nil {
			u.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s (stat err: %v)",
				item.App, name, item.Path, err)
		} else {
			u.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s (%s Activity Queue status: %v)",
				item.App, name, item.Path, item.App, item.IDs["reason"])
		}

		return
	}

	if item.Syncthing {
		if tmpFile := u.hasSyncThingFile(item.Path); tmpFile != "" {
			u.Printf("[%s] Completed item still syncing: %s, found Syncthing .tmp file: %s", item.App, name, tmpFile)
			return
		}
	}

	// This updates the item in the map.
	item.Status = QUEUED
	item.Updated = time.Now()
	queueSize, _ := u.Extract(&xtractr.Xtract{
		Password:  u.getPasswordFromPath(item.Path),
		Passwords: u.Passwords,
		Name:      name,
		Filter: xtractr.Filter{
			Path: item.Path,
			ExcludeSuffix: xtractr.AllExcept([]string{
				".rar", ".r00", ".zip", ".7z", ".gz", ".tgz", ".tar", ".tar.gz", ".bz2", ".tbz2",
			}),
		},
		TempFolder: false,
		DeleteOrig: false,
		CBChannel:  u.updates,
	})

	u.logQueuedDownload(queueSize, item, files)
}

func (u *Unpackerr) logQueuedDownload(queueSize int, item *Extract, files xtractr.ArchiveList) {
	count := fmt.Sprint("1 archive: ", files.Random()[0])
	if fileCount := files.Count(); fileCount > 1 {
		count = fmt.Sprintf("%v archives in %d folders", fileCount, len(files))
	}

	u.Printf("[%s] Extraction Queued: %s, retries: %d, %s, delete orig: %v, queue size: %d",
		item.App, item.Path, item.Retries, count, item.DeleteOrig, queueSize)
	u.updateHistory(string(item.App) + ": " + item.Path)
}

// saveCompletedDownload stores a completed download in the internal unpackerr map.
// This is called from the app methods.
func (u *Unpackerr) saveCompletedDownload(name string, x *Extract) {
	if _, ok := u.Map[name]; !ok {
		x.Updated = time.Now()
		u.Map[name] = x
	}
}

func (u *Unpackerr) getPasswordFromPath(s string) string {
	start, end := strings.Index(s, "{{"), strings.Index(s, "}}")

	if start == -1 || end == -1 || start > end {
		return ""
	}

	u.Debugf("Found password in Path: %s", s[start+2:end])

	return s[start+2 : end]
}

// checkExtractDone checks if an extracted and imported item needs to be deleted.
// Or if an extraction failed and needs to be restarted.
// This runs at a short interval to check for extraction state changes, and should return quickly.
//
//nolint:cyclop,wsl
func (u *Unpackerr) checkExtractDone() {
	for name, data := range u.Map {
		switch elapsed := time.Since(data.Updated); {
		case data.Status == DELETED && elapsed >= data.DeleteDelay:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			delete(u.Map, name)
			u.Printf("[%s] Finished, Removed History: %v", data.App, name)
		case data.App == FolderString:
			continue // folders are handled in folder.go.
		case data.Status == EXTRACTFAILED && elapsed >= u.RetryDelay.Duration &&
			(u.MaxRetries == 0 || data.Retries < u.MaxRetries):
			u.Retries++
			data.Retries++
			data.Status = WAITING
			data.Updated = time.Now()
			u.Printf("[%s] Extract failed %v ago, triggering restart (%d/%d): %v",
				data.App, elapsed.Round(time.Second), data.Retries, u.MaxRetries, name)
		case data.Status == IMPORTED && elapsed >= data.DeleteDelay:
			var webhook bool

			if data.DeleteOrig {
				u.delChan <- &fileDeleteReq{Paths: []string{data.Path}}
				webhook = true
			} else if data.Resp != nil && len(data.Resp.NewFiles) > 0 && data.DeleteDelay >= 0 {
				// In a routine so it can run slowly and not block.
				u.delChan <- &fileDeleteReq{Paths: data.Resp.NewFiles, PurgeEmptyParent: true}
				webhook = true
			}

			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: data.Resp}, webhook)
		}
	}
}

// handleXtractrCallback handles callbacks from the xtractr library for starr apps (not folders).
// This takes the provided info and logs it then sends it the queue update method.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) {
	if item := u.Map[resp.X.Name]; resp.Done && item != nil {
		u.updateMetrics(resp, item.App, item.URL)
	}

	switch {
	case !resp.Done:
		u.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTING, Resp: resp}, true)
	case resp.Error != nil:
		u.Errorf("Extraction Failed: %s: %v", resp.X.Name, resp.Error)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: resp}, true)
	default:
		u.Printf("Extraction Finished: %s => elapsed: %v, archives: %d, extra archives: %d, "+
			"files extracted: %d, wrote: %dMiB", resp.X.Name, resp.Elapsed.Round(time.Second),
			len(resp.Archives), len(resp.Extras), len(resp.NewFiles), resp.Size/mebiByte)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTED, Resp: resp}, true)
	}
}

// Looking for a message that looks like:
// "No files found are eligible for import in /downloads/Downloading/Space.Warriors.S99E88.GrOuP.1080p.WEB.x264".
func (u *Unpackerr) getDownloadPath(outputPath string, app starr.App, title string, paths []string) string {
	var errs []error

	// Try all the user provided paths.
	for _, path := range paths {
		path = filepath.Join(path, title)

		switch _, err := os.Stat(path); err {
		default:
			errs = append(errs, err)
		case nil:
			return path
		}
	}

	// Print the errors for each user-provided path.
	u.Debugf("%s: Errors encountered looking for %s path: %q", app, title, errs)

	if outputPath != "" {
		u.Debugf("%s: Configured paths do not exist; trying 'outputPath': %s", app, outputPath)
		return outputPath
	}

	u.Debugf("%s: Configured paths do not exist and 'outputPath' is empty for: %s", app, title)

	return filepath.Join(paths[0], title) // useless, but return something. :(
}

// isComplete is run so many times in different places that is became a method.
func (u *Unpackerr) isComplete(status, protocol, protos string) bool {
	for _, s := range strings.Fields(strings.ReplaceAll(protos, ",", " ")) {
		if strings.EqualFold(protocol, s) {
			return strings.EqualFold(status, "completed")
		}
	}

	return false
}

// added for https://github.com/davidnewhall/unpackerr/issues/235
func (u *Unpackerr) hasSyncThingFile(dirPath string) string {
	files, _ := u.Xtractr.GetFileList(dirPath)
	for _, file := range files {
		if strings.HasSuffix(file, ".tmp") {
			return file
		}
	}

	return ""
}
