package unpacker

import (
	"log"
	"time"

	"golift.io/xtractr"
)

// handleItemFinishedImport checks if sonarr/radarr/lidarr files should be deleted.
func (u *Unpackerr) handleFinishedImport(data *Extracts, name string) {
	switch elapsed := time.Since(data.Updated); {
	case data.Status == IMPORTED:
		u.DeLogf("%v: Awaiting Delete Delay (%v remains): %v",
			data.App, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
		return
	case elapsed >= u.DeleteDelay.Duration:
		u.Map[name].Status = DELETED

		DeleteFiles(data.Files...)
	default:
		u.Map[name].Status = IMPORTED

		log.Printf("[%v] Imported: %v (delete in %v)", data.App, name, u.DeleteDelay)
	}

	u.Map[name].Updated = time.Now()
}

// handleCompletedDownload checks if a sonarr/radarr/lidar completed item needs to be extracted.
func (u *Unpackerr) handleCompletedDownload(name, app, path string) {
	files := xtractr.FindCompressedFiles(path)
	if len(files) == 0 {
		u.DeLogf("%s: Completed item still in queue: %s, no extractable files found at: %s", app, name, path)
		return
	}

	log.Printf("[%s] Found %d extractable item(s): %s (%s)", app, len(files), name, path)

	u.Map[name] = &Extracts{
		Path:    path,
		App:     app,
		Status:  QUEUED,
		Updated: time.Now(),
	}

	queueSize, err := u.Extract(&xtractr.Xtract{
		Name:       name,
		SearchPath: path,
		TempFolder: false,
		DeleteOrig: false,
		CBFunction: u.handleXtractrCallback,
		FindFileEx: []xtractr.ExtType{xtractr.RAR},
	})
	if err != nil {
		log.Println("[ERROR]", err)
		return
	}

	log.Printf("[%s] Extraction Queued: %s, queue size: %d", app, path, queueSize)
}

// handleXtractrCallback handles callbacks from the xtractr library for onarr/radarr/lidar.
// This takes the provided info and logs it then sends it into the update channel.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) {
	const mebiByte = 1024 * 1024

	switch {
	case !resp.Done:
		log.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTING}
	case resp.Error != nil:
		log.Printf("Extraction Error: %s: %v", resp.X.Name, resp.Error)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTFAILED}
	default: // this runs in a go routine
		log.Printf("Extraction Finished: %s => elapsed: %d, archives: %d, "+
			"extra archives: %d, files extracted: %d, MiB written: %d",
			resp.X.Name, resp.Elapsed, len(resp.Archives), len(resp.Extras),
			len(resp.AllFiles), resp.Size/mebiByte)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTED, Files: resp.NewFiles}
	}
}
