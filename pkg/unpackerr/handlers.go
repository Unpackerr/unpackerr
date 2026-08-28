package unpackerr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

// Extract holds data for files being extracted.
type Extract struct {
	Syncthing   bool
	SplitFlac   bool
	Retries     uint
	Path        string // Local path (resolved for extraction on this host).
	OutputPath  string // Original path from Starr app (may be UNC/remote — used for ManualImport).
	App         starr.App
	URL         string
	Updated     time.Time
	DeleteDelay time.Duration
	DeleteOrig  bool
	Status      ExtractStatus
	IDs         map[string]any
	Resp        *xtractr.Response
	XProg       *ExtractProgress
	// PreFiles maps top-level basenames present in Path before extraction to
	// their Lstat info (may be nil if stat failed). Distinguishes download
	// content from interrupted-extraction remnants, including case-only
	// spelling differences on case-insensitive filesystems via os.SameFile.
	PreFiles map[string]os.FileInfo
}

// StarrConfig is the shared config items for all starr apps.
type StarrConfig struct {
	starr.Config
	Path        string        `json:"path"         toml:"path"         xml:"path"         yaml:"path"`
	Paths       StringSlice   `json:"paths"        toml:"paths"        xml:"paths"        yaml:"paths"`
	Protocols   string        `json:"protocols"    toml:"protocols"    xml:"protocols"    yaml:"protocols"`
	DeleteOrig  bool          `json:"delete_orig"  toml:"delete_orig"  xml:"delete_orig"  yaml:"delete_orig"`
	DeleteDelay cnfg.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Syncthing   bool          `json:"syncthing"    toml:"syncthing"    xml:"syncthing"    yaml:"syncthing"`
	ValidSSL    bool          `json:"valid_ssl"    toml:"valid_ssl"    xml:"valid_ssl"    yaml:"valid_ssl"`
	Timeout     cnfg.Duration `json:"timeout"      toml:"timeout"      xml:"timeout"      yaml:"timeout"`
}

// checkQueueChanges checks each item for state changes from the app queues.
func (u *Unpackerr) checkQueueChanges(now time.Time) {
	for name, data := range u.Map {
		switch {
		case data.App == FolderString:
			continue // folders are handled in folder.go.
		case !u.haveQitem(name, data.App):
			// This fires when an items becomes missing (imported/deleted) from the application queue.
			switch elapsed := now.Sub(data.Updated); {
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
				u.updateQueueStatus(&newStatus{Name: name, Status: IMPORTED, Resp: data.Resp}, now, true)
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
			data.Updated = now
			data.PreFiles = nil // new cycle; snapshot again on the next extract.
		}

		u.Printf("[%s] Status: %s (%v, elapsed: %v) %s", data.App, name, data.Status.Desc(),
			now.Sub(data.Updated).Round(time.Second), data.XProg)
	}
}

// extractCompletedDownloads process each download and checks if it needs to be extracted.
// This is called from the main go routine in start.go and it only processes starr apps, not folders.
func (u *Unpackerr) extractCompletedDownloads(now time.Time) {
	for name, item := range u.Map {
		if item.App != FolderString && item.Status < QUEUED {
			u.extractCompletedDownload(name, now, item)
		}
	}
}

// extractCompletedDownload checks if a completed starr download needs to be queued for extraction.
// This is called by extractCompletedDownloads() via the main routine in start.go.
func (u *Unpackerr) extractCompletedDownload(name string, now time.Time, item *Extract) {
	if d := u.StartDelay.Duration - now.Sub(item.Updated); d > time.Second { // wiggle room.
		u.Printf("[%s] Waiting for Start Delay: %v (%v remains)", item.App, name, d.Round(time.Second))
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

	// Snapshot once per queue item. Retries must not fold leftovers that
	// failed to clear into download content.
	item.PreFiles = keepDirSnapshot(item.PreFiles, item.Path)
	item.Status = QUEUED
	item.Updated = now
	// This queues the extraction. Which may start right away.
	archiveTypes := []string{".rar", ".r00", ".zip", ".7z", ".7z.001", ".gz", ".tgz", ".tar", ".tar.gz", ".bz2", ".tbz2"}
	if item.SplitFlac {
		archiveTypes = append(archiveTypes, ".cue")
	}

	queueSize, _ := u.Extract(&xtractr.Xtract{
		Password:      u.getPasswordFromPath(item.Path),
		Passwords:     u.Passwords,
		Name:          name,
		Path:          item.Path,
		ExcludeSuffix: xtractr.AllExcept(archiveTypes...),
		TempFolder:    false,
		DeleteOrig:    false,
		CBChannel:     u.updates,
		Progress:      u.progressUpdateCallback(item),
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

func (u *Unpackerr) getPasswordFromPath(path string) string {
	start, end := strings.Index(path, "{{"), strings.Index(path, "}}")

	if start == -1 || end == -1 || start > end {
		return ""
	}

	u.Debugf("Found password in Path: %s", path[start+2:end])

	return path[start+2 : end]
}

// checkExtractDone checks if an extracted and imported item needs to be deleted.
// Or if an extraction failed and needs to be restarted.
// This runs at a short interval to check for extraction state changes, and should return quickly.
//
//nolint:cyclop,wsl
func (u *Unpackerr) checkExtractDone(now time.Time) {
	for name, item := range u.Map {
		switch elapsed := now.Sub(item.Updated); {
		case item.Status == DELETED && elapsed >= item.DeleteDelay:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			delete(u.Map, name)
			u.Printf("[%s] Finished, Removed History: %v", item.App, name)
		case item.App == FolderString:
			continue // folders are handled in folder.go.
		case item.Status == EXTRACTFAILED && elapsed >= u.RetryDelay.Duration &&
			(u.MaxRetries == 0 || item.Retries < u.MaxRetries):
			u.Retries++
			item.Retries++
			item.Status = WAITING
			item.Updated = now
			u.Printf("[%s] Extract failed %v ago, triggering restart (%d/%d): %v",
				item.App, elapsed.Round(time.Second), item.Retries, u.MaxRetries, name)
		case item.Status == EXTRACTFAILED && u.MaxRetries > 0 && item.Retries >= u.MaxRetries:
			// Retries exhausted — clean up to prevent the item from staying in the map forever.
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: item.Resp}, now, true)
			u.Printf("[%s] Retries exhausted (%d/%d), giving up: %v",
				item.App, item.Retries, u.MaxRetries, name)
		case (item.Status == EXTRACTED || item.Status == EXTRACTING || item.Status == QUEUED) &&
			elapsed >= staleItemTimeout:
			// Safety net: items stuck at intermediate states for too long are cleaned up
			// to prevent unbounded map growth (e.g. Starr app never imports the item).
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: item.Resp}, now, true)
			u.Printf("[%s] Stale item removed after %v at status %s: %v",
				item.App, elapsed.Round(time.Second), item.Status.Desc(), name)
		case item.Status == IMPORTED && elapsed >= item.DeleteDelay:
			var webhook bool

			if item.DeleteOrig {
				u.delChan <- &fileDeleteReq{Paths: []string{item.Path}}
				webhook = true //nolint:wsl_v5
			} else if item.Resp != nil && len(item.Resp.NewFiles) > 0 && item.DeleteDelay >= 0 {
				// Delete extracted files and purge empty parents up to and including the download path.
				u.delChan <- &fileDeleteReq{
					Paths:            item.Resp.NewFiles,
					PurgeEmptyParent: true,
					PurgeEmptyRoot:   item.Path,
				}
				webhook = true //nolint:wsl_v5
			}

			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: item.Resp}, now, webhook)
		}
	}
}

// handleXtractrCallback handles callbacks from the xtractr library for starr apps (not folders).
// This takes the provided info and logs it then sends it the queue update method.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) {
	item := u.Map[resp.X.Name]
	if resp.Done && item != nil {
		u.updateMetrics(resp, item.App, item.URL)
	} else if item != nil {
		item.XProg.Archives = resp.Archives.Count() + resp.Extras.Count()
	}

	switch now := resp.Started.Add(resp.Elapsed); {
	case !resp.Done:
		u.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTING, Resp: resp}, now, true)
	case u.handleStarrRefused(item, resp, now):
		return
	case resp.Error != nil:
		u.Errorf("Extraction Failed: %s: %v", resp.X.Name, resp.Error)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: resp}, now, true)
	default:
		files := fileList(resp.X.Path)
		u.Printf("Extraction Finished: %s => elapsed: %v, archives: %d, extra archives: %d, "+
			"files extracted: %d, wrote: %sB", resp.X.Name, resp.Elapsed.Round(time.Second),
			resp.Archives.Count(), resp.Extras.Count(), len(resp.NewFiles), bytefmt.ByteSize(resp.Size))
		u.Debugf("Extraction Finished: %d files in path: %s", len(files), files)

		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTED, Resp: resp}, now, true)

		if item != nil && item.App == starr.Lidarr && item.SplitFlac && resp.Size > 0 {
			go u.importSplitFlacTracks(item, u.lidarrServerByURL(item.URL))
		}
	}
}

// handleStarrRefused applies remnant_action on any terminal response that
// reported occupied destinations, including those that also carry an error.
// Returns true when the item's status was fully handled.
func (u *Unpackerr) handleStarrRefused(item *Extract, resp *xtractr.Response, now time.Time) bool {
	if item == nil || len(resp.Refused) == 0 {
		return false
	}

	restart, fail := u.applyRefused(item.PreFiles, []string{item.Path}, resp, item.Retries)
	switch {
	case fail:
		u.Errorf("[%s] Extraction blocked by interrupted-extraction remnant(s): %s",
			item.App, resp.X.Name)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: resp}, now, true)

		return true
	case restart && resp.Error != nil:
		u.Printf("[%s] Cleared interrupted-extraction remnant(s); extraction still failed: %s",
			item.App, resp.X.Name)

		return false // EXTRACTFAILED from the original error; remnants are already gone.
	case restart:
		u.Retries++
		item.Retries++
		item.Status = WAITING
		item.Updated = now
		item.Resp = resp
		u.Printf("[%s] Cleared interrupted-extraction remnant(s), restarting extraction: %s",
			item.App, resp.X.Name)

		return true
	default:
		return false
	}
}

// keepDirSnapshot returns the first pre-extraction listing for this item.
// Retries must not recapture leftovers that failed to clear.
func keepDirSnapshot(existing map[string]os.FileInfo, path string) map[string]os.FileInfo {
	if existing != nil {
		return existing
	}

	return snapshotDir(path)
}

func snapshotDir(path string) map[string]os.FileInfo {
	names := fileList(path)
	out := make(map[string]os.FileInfo, len(names))

	for _, name := range names {
		info, err := os.Lstat(filepath.Join(path, name))
		if err != nil {
			out[name] = nil
			continue
		}

		out[name] = info
	}

	return out
}

func isDownloadContent(preFiles map[string]os.FileInfo, dest string) bool {
	if _, ok := preFiles[filepath.Base(dest)]; ok {
		return true
	}

	destInfo, err := os.Lstat(dest)
	if err != nil {
		return false
	}

	for _, info := range preFiles {
		if info != nil && os.SameFile(info, destInfo) {
			return true
		}
	}

	return false
}

func underDestRoots(dest string, roots []string) bool {
	dest = filepath.Clean(dest)

	for _, root := range roots {
		if root == "" {
			continue
		}

		root = filepath.Clean(root)
		if dest == root || strings.HasPrefix(dest, root+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

func (u *Unpackerr) remnantActionValue() string {
	if u.RemnantAction == "" {
		return remnantActionRename
	}

	return u.RemnantAction
}

func (u *Unpackerr) refusedRemnants(
	preFiles map[string]os.FileInfo,
	destRoots []string,
	resp *xtractr.Response,
) []string {
	if resp == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(resp.Refused))

	var remnants []string

	for _, refused := range resp.Refused {
		if _, dup := seen[refused.Dest]; dup {
			continue
		}

		seen[refused.Dest] = struct{}{}

		if isDownloadContent(preFiles, refused.Dest) {
			u.Debugf("Keeping download file that blocked extraction: %s", refused.Dest)
			continue
		}

		if !underDestRoots(refused.Dest, destRoots) {
			u.Debugf("Ignoring refused path outside extract dest: %s", refused.Dest)
			continue
		}

		remnants = append(remnants, refused.Dest)
	}

	return remnants
}

// applyRefused classifies Response.Refused against the pre-extraction snapshot.
// The first bool is restart (every remnant was cleared; re-queue). The second is
// fail (any remnant remains; do not report EXTRACTED).
func (u *Unpackerr) applyRefused(
	preFiles map[string]os.FileInfo,
	destRoots []string,
	resp *xtractr.Response,
	retries uint,
) (bool, bool) {
	remnants := u.refusedRemnants(preFiles, destRoots, resp)
	if len(remnants) == 0 {
		return false, false
	}

	if u.remnantActionValue() == remnantActionOff {
		for _, dest := range remnants {
			u.Printf("Interrupted-extraction remnant left in place (remnant_action=off): %s", dest)
		}

		return false, false
	}

	if u.MaxRetries == 0 || retries < u.MaxRetries {
		cleared := 0

		for _, dest := range remnants {
			if u.clearRemnant(dest) {
				cleared++
			}
		}

		if cleared == len(remnants) {
			return true, false
		}
	}

	return false, true
}

// repairRemnants removes or renames files that blocked extraction and did
// not arrive with the download. Returns the count of blockers cleared.
func (u *Unpackerr) repairRemnants(preFiles map[string]os.FileInfo, destRoots []string, resp *xtractr.Response) int {
	cleared := 0

	for _, dest := range u.refusedRemnants(preFiles, destRoots, resp) {
		if u.clearRemnant(dest) {
			cleared++
		}
	}

	return cleared
}

func (u *Unpackerr) clearRemnant(dest string) bool {
	info, err := os.Lstat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return true // already gone; retry the extract into the empty dest.
		}

		u.Errorf("Checking interrupted-extraction remnant %s: %v", dest, err)

		return false
	}

	switch u.remnantActionValue() {
	case remnantActionOff:
		u.Printf("Interrupted-extraction remnant left in place (remnant_action=off): %s", dest)
		return false
	case remnantActionDelete:
		remove := os.Remove
		if info.IsDir() {
			remove = os.RemoveAll
		}

		if err := remove(dest); err != nil {
			u.Errorf("Removing interrupted-extraction remnant %s: %v", dest, err)
			return false
		}

		u.Printf("Removed interrupted-extraction remnant: %s", dest)

		return true
	default: // rename
		target, err := unusedRemnantPath(dest)
		if err != nil {
			u.Errorf("Renaming interrupted-extraction remnant %s: %v", dest, err)
			return false
		}

		if err := os.Rename(dest, target); err != nil {
			u.Errorf("Renaming interrupted-extraction remnant %s -> %s: %v", dest, target, err)
			return false
		}

		u.Printf("Renamed interrupted-extraction remnant: %s -> %s (you may delete it)", dest, target)

		return true
	}
}

func unusedRemnantPath(path string) (string, error) {
	for n := range maxRemnantCopies {
		candidate := path + remnantSuffix
		if n > 0 {
			candidate = fmt.Sprintf("%s%s.%d", path, remnantSuffix, n)
		}

		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}

		if err != nil {
			return "", fmt.Errorf("os.Lstat: %w", err)
		}
	}

	return "", fmt.Errorf("%w: %s", errNoRemnantName, path)
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

	// The title often differs from the actual folder name (e.g. torrent names include genre tags).
	// Try the folder name from outputPath against configured paths — the folder name is the real
	// directory on disk. This also handles cross-platform setups where outputPath is a UNC/Windows
	// path but the configured paths are local Linux mounts of the same share.
	if outputPath != "" {
		outputFolder := filepath.Base(filepath.FromSlash(strings.ReplaceAll(outputPath, `\`, `/`)))
		if outputFolder != "" && outputFolder != "." && outputFolder != title {
			for _, path := range paths {
				candidate := filepath.Join(path, outputFolder)

				if _, err := os.Stat(candidate); err == nil {
					u.Debugf("%s: Resolved via outputPath folder name: %s -> %s", app, outputPath, candidate)
					return candidate
				}
			}
		}

		u.Debugf("%s: Configured paths do not exist; trying 'outputPath': %s", app, outputPath)

		return outputPath
	}

	u.Debugf("%s: Configured paths do not exist and 'outputPath' is empty for: %s", app, title)

	return filepath.Join(paths[0], title) // useless, but return something. :(
}

// isComplete is run so many times in different places that it became a method.
func (u *Unpackerr) isComplete(status string, protocol starr.Protocol, protos string) bool {
	for s := range strings.FieldsSeq(strings.ReplaceAll(protos, ",", " ")) {
		if strings.EqualFold(string(protocol), s) {
			return strings.EqualFold(status, "completed")
		}
	}

	return false
}

// added for https://github.com/Unpackerr/unpackerr/issues/235
func (u *Unpackerr) hasSyncThingFile(dirPath string) string {
	files, _ := u.GetFileList(dirPath)
	for _, file := range files {
		if strings.HasSuffix(file, ".tmp") {
			return file
		}
	}

	return ""
}
