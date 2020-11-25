package unpackerr

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

// Extract holds data for files being extracted.
type Extract struct {
	Path    string
	App     string
	IDs     map[string]interface{}
	Status  ExtractStatus
	Updated time.Time
	Resp    *xtractr.Response
}

// checkImportsDone checks if extracted items have been imported.
func (u *Unpackerr) checkImportsDone() {
	for name, data := range u.Map {
		switch {
		case data.Status > IMPORTED || data.Status == EXTRACTING || data.Status == EXTRACTFAILED:
			continue
		case !u.haveQitem(name, data.App):
			// We only want finished items.
			u.handleFinishedImport(data, name)
		case data.Status == IMPORTED:
			// The item fell out of the app queue and came back. Reset it.
			u.Printf("%s: Resetting: %s - De-queued and returned", data.App, name)
			data.Status = WAITING
			data.Updated = time.Now()
		}

		u.Debugf("%s: Status: %s (%v, elapsed: %v)", data.App, name, data.Status.Desc(),
			time.Since(data.Updated).Round(time.Second))
	}
}

// handleItemFinishedImport checks if sonarr/radarr/lidarr files should be deleted.
func (u *Unpackerr) handleFinishedImport(data *Extract, name string) {
	switch elapsed := time.Since(data.Updated); {
	case data.Status == WAITING:
		// A waiting item just imported. We never extracted it. Remove it and move on.
		delete(u.Map, name)
		u.Printf("[%v] Imported: %v (not extracted, removing from history)", data.App, name)
	case data.Status > IMPORTED:
		u.Debugf("Already imported? %s", name)
	case data.Status == IMPORTED && elapsed+time.Millisecond >= u.DeleteDelay.Duration:
		// In a routine so it can run slowly and not block.
		go u.DeleteFiles(data.Resp.NewFiles...)
		u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: data.Resp})
	case data.Status == IMPORTED:
		u.Debugf("%v: Awaiting Delete Delay (%v remains): %v",
			data.App, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
	case data.Status != IMPORTED:
		u.updateQueueStatus(&newStatus{Name: name, Status: IMPORTED, Resp: data.Resp})
		u.Printf("[%v] Imported: %v (delete in %v)", data.App, name, u.DeleteDelay)
	}
}

// handleCompletedDownload checks if a sonarr/radarr/lidar completed item needs to be extracted.
func (u *Unpackerr) handleCompletedDownload(name, app, path string, ids map[string]interface{}) {
	item, ok := u.Map[name]
	if !ok {
		u.Map[name] = &Extract{
			App:     app,
			Path:    path,
			IDs:     ids,
			Status:  WAITING,
			Updated: time.Now(),
		}
		item = u.Map[name]
	}

	if time.Since(item.Updated) < u.Config.StartDelay.Duration {
		u.Printf("[%s] Waiting for Start Delay: %v (%v remains)", app, name,
			u.Config.StartDelay.Duration-time.Since(item.Updated).Round(time.Second))

		return
	}

	files := xtractr.FindCompressedFiles(path)
	if len(files) == 0 {
		_, err := os.Stat(path)
		u.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s (stat err: %v)",
			app, name, path, err)

		return
	}

	item.Status = QUEUED
	item.Updated = time.Now()

	queueSize, _ := u.Extract(&xtractr.Xtract{
		Name:       name,
		SearchPath: path,
		TempFolder: false,
		DeleteOrig: false,
		CBChannel:  u.updates,
	})
	u.Printf("[%s] Extraction Queued: %s, extractable files: %d, items in queue: %d", app, path, len(files), queueSize)
}

// checkExtractDone checks if an extracted item imported items needs to be deleted.
// Or if an extraction failed and needs to be restarted.
func (u *Unpackerr) checkExtractDone() {
	for name, data := range u.Map {
		switch elapsed := time.Since(data.Updated); {
		case data.Status == EXTRACTFAILED && elapsed >= u.RetryDelay.Duration:
			u.Restarted++
			delete(u.Map, name)
			u.Printf("[%s] Extract failed %v ago, removed history so it can be restarted: %v",
				data.App, elapsed.Round(time.Second), name)
		case data.Status == DELETED && elapsed >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			delete(u.Map, name)
			u.Printf("[%s] Finished, Removed History: %v", data.App, name)
		}
	}
}

// handleXtractrCallback handles callbacks from the xtractr library for sonarr/radarr/lidarr.
// This takes the provided info and logs it then sends it the queue update method.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) {
	switch {
	case !resp.Done:
		u.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTING, Resp: resp})
	case resp.Error != nil:
		u.Printf("Extraction Error: %s: %v", resp.X.Name, resp.Error)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTFAILED, Resp: resp})
	default:
		u.Printf("Extraction Finished: %s => elapsed: %v, archives: %d, extra archives: %d, "+
			"files extracted: %d, wrote: %dMiB", resp.X.Name, resp.Elapsed.Round(time.Second),
			len(resp.Archives), len(resp.Extras), len(resp.NewFiles), resp.Size/mebiByte)
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Status: EXTRACTED, Resp: resp})
	}
}

// Looking for a message that looks like:
// "No files found are eligible for import in /downloads/Downloading/Space.Warriors.S99E88.GrOuP.1080p.WEB.x264".
func (u *Unpackerr) getDownloadPath(s []starr.StatusMessage, app, title, path string) string {
	var err error

	path = filepath.Join(path, title)
	if _, err = os.Stat(path); err == nil {
		u.Debugf("%s: Configured path exists: %s", app, path)

		return path // the server path exists, so use that.
	}

	for _, m := range s {
		if m.Title != title {
			continue
		}

		for _, msg := range m.Messages {
			if strings.HasPrefix(msg, prefixPathMsg) && strings.HasSuffix(msg, title) {
				newPath := strings.TrimSpace(strings.TrimPrefix(msg, prefixPathMsg))
				u.Debugf("%s: Configured path (%s, err: %v) does not exist; trying path found in status message: %s",
					app, path, err, newPath)

				return newPath
			}
		}
	}

	u.Debugf("%s: Configured path does not exist (err: %v), and could not find alternative path in error message: %s ",
		app, err, path)

	return path
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
