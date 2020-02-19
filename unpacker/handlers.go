package unpacker

import (
	"log"
	"time"

	"golift.io/xtractr"
)

// handleItemFinishedImport checks if sonarr/radarr/lidarr files should be deleted.
func (u *Unpackerr) handleFinishedImport(data *Extracts, name string) {
	elapsed := time.Since(data.Updated)

	switch {
	case data.Status == DOWNLOADING:
		// A waiting item just imported. We never extracted it. Remove it and move on.
		delete(u.Map, name)
		log.Printf("[%v] Imported: %v (not extracted, removing from history)", data.App, name)
	case data.Status > IMPORTED:
		return
	case data.Status == IMPORTED && elapsed+time.Millisecond > u.DeleteDelay.Duration:
		u.Map[name].Status = DELETED
		u.Map[name].Updated = time.Now()

		// In a routine so it can run slowly and not block.
		go DeleteFiles(data.Files...)
	case data.Status == IMPORTED:
		u.DeLogf("%v: Awaiting Delete Delay (%v remains): %v",
			data.App, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
	case data.Status != IMPORTED:
		u.Map[name].Status = IMPORTED
		u.Map[name].Updated = time.Now()

		log.Printf("[%v] Imported: %v (delete in %v)", data.App, name, u.DeleteDelay)
	}
}

// handleCompletedDownload checks if a sonarr/radarr/lidar completed item needs to be extracted.
func (u *Unpackerr) handleCompletedDownload(name, app, path string) {
	item, ok := u.Map[name]
	if !ok {
		u.Map[name] = &Extracts{
			Path:    path,
			App:     app,
			Status:  DOWNLOADING,
			Updated: time.Now(),
		}
		item = u.Map[name]
	}

	if time.Since(item.Updated) < u.Config.StartDelay.Duration {
		u.DeLogf("%s: Item Waiting for Start Delay: %v", app, name)
		return
	}

	files := xtractr.FindCompressedFiles(path)
	if len(files) == 0 {
		log.Printf("[%s] Completed item still waiting: %s, no extractable files found at: %s", app, name, path)
		return
	}

	log.Printf("[%s] Found %d extractable item(s): %s (%s)", app, len(files), name, path)

	item.Status = QUEUED
	item.Updated = time.Now()

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

	log.Printf("[%s] Extraction Queued: %s, items in queue: %d", app, path, queueSize)
}

// handleXtractrCallback handles callbacks from the xtractr library for onarr/radarr/lidar.
// This takes the provided info and logs it then sends it into the update channel.
func (u *Unpackerr) handleXtractrCallback(resp *xtractr.Response) {
	switch {
	case !resp.Done:
		log.Printf("Extraction Started: %s, items in queue: %d", resp.X.Name, resp.Queued)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTING}
	case resp.Error != nil:
		log.Printf("Extraction Error: %s: %v", resp.X.Name, resp.Error)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTFAILED}
	default: // this runs in a go routine
		log.Printf("Extraction Finished: %s => elapsed: %v, archives: %d, "+
			"extra archives: %d, files extracted: %d, wrote: %dMiB",
			resp.X.Name, resp.Elapsed.Round(time.Second), len(resp.Archives), len(resp.Extras),
			len(resp.AllFiles), resp.Size/mebiByte)
		u.updates <- &Extracts{Path: resp.X.Name, Status: EXTRACTED, Files: resp.NewFiles}
	}
}
