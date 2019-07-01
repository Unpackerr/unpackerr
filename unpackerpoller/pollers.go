package unpackerpoller

import (
	"log"
	"path/filepath"
	"time"
)

// PollDeluge at an interval and save the transfer list to r.Deluge
func (u *UnpackerPoller) PollDeluge() error {
	var err error
	u.Xfers.Lock()
	defer u.Xfers.Unlock()
	if u.Xfers.Map, err = u.Deluge.GetXfersCompat(); err != nil {
		log.Println("Deluge Error:", err)
		return err
	}
	log.Println("Deluge Updated:", len(u.Xfers.Map), "Transfers")
	return nil
}

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (u *UnpackerPoller) PollSonarr() {
	var err error
	u.SonarrQ.Lock()
	defer u.SonarrQ.Unlock()
	if u.SonarrQ.List, err = u.Sonarr.SonarrQueue(); err != nil {
		log.Println("Sonarr Error:", err)
	} else {
		log.Println("Sonarr Updated:", len(u.SonarrQ.List), "Items Queued")
	}
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (u *UnpackerPoller) PollRadarr() {
	var err error
	u.RadarrQ.Lock()
	defer u.RadarrQ.Unlock()
	if u.RadarrQ.List, err = u.Radarr.RadarrQueue(); err != nil {
		log.Println("Radarr Error:", err)
	} else {
		log.Println("Radarr Updated:", len(u.RadarrQ.List), "Items Queued")
	}
}

// PollChange runs other tasks.
// Those tasks: a) look for things to extract, b) look for things to delete.
func (u *UnpackerPoller) PollChange() {
	// Don't start this for 2 whole minutes.
	time.Sleep(time.Minute)
	log.Println("Starting Cleanup Routine (interval: 1 minute)")
	// This runs more often because of the cleanup tasks.
	// It doesn't poll external data, unless it finds something to extract.
	ticker := time.NewTicker(time.Minute).C
	for range ticker {
		if u.Xfers.Map == nil {
			continue // No data.
		}
		u.CheckExtractDone()
		if u.SonarrQ.List != nil {
			u.CheckSonarrQueue()
		}
		if u.RadarrQ.List != nil {
			u.CheckRadarrQueue()
		}
	}
}

// CheckExtractDone checks if an extracted item has been imported.
func (u *UnpackerPoller) CheckExtractDone() {
	log.Printf("Extract Statuses: %d actively extracting, %d queued, "+
		"%d extracted, %d imported, %d failed, %d deleted",
		u.eCount().extracting, u.eCount().queued, u.eCount().extracted,
		u.eCount().imported, u.eCount().failed, u.eCount().deleted)
	for name, data := range u.GetHistory() {
		u.DeLogf("Extract Status: %v (status: %v, elapsed: %v)", name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))
		switch elapsed := time.Since(u.GetStatus(name).Updated); {
		case data.Status >= DELETED && elapsed >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			log.Printf("%v: Removing History: %v", data.App, name)
			u.DeleteStatus(name)
		case data.Status < EXTRACTED || data.Status > IMPORTED:
			// Only process items that have finished extraction and are not deleted.
			continue
		case data.App == "Sonarr":
			if q := u.getSonarQitem(name); q.Status == "" {
				u.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed)
			} else {
				u.DeLogf("Sonarr Item Waiting For Import: %v -> %v", name, q.Status)
			}
		case data.App == "Radarr":
			if q := u.getRadarQitem(name); q.Status == "" {
				u.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed)
			} else {
				u.DeLogf("Radarr Item Waiting For Import: %v -> %v", name, q.Status)
			}
		}
	}
}

// HandleExtractDone checks if files should be deleted.
func (u *UnpackerPoller) HandleExtractDone(app, name string, status ExtractStatus, files []string, elapsed time.Duration) {
	switch {
	case status != IMPORTED:
		log.Printf("%v Imported: %v (delete in %v)", app, name, u.DeleteDelay)
		u.UpdateStatus(name, IMPORTED, nil)
	case elapsed >= u.DeleteDelay.Duration:
		go func() {
			status := DELETED
			if err := deleteFiles(files); err != nil {
				status = DELETEFAILED
			}
			u.UpdateStatus(name, status, nil)
		}()
	default:
		u.DeLogf("%v: Awaiting Delete Delay (%v remains): %v", app, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *UnpackerPoller) CheckSonarrQueue() {
	u.SonarrQ.RLock()
	defer u.SonarrQ.RUnlock()
	for _, q := range u.SonarrQ.List {
		if q.Status == "Completed" {
			u.HandleCompleted(q.Title, "Sonarr")
		} else {
			u.DeLogf("Sonarr: %v (%d%%): %v (Ep: %v)", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
		}
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *UnpackerPoller) CheckRadarrQueue() {
	u.RadarrQ.RLock()
	defer u.RadarrQ.RUnlock()
	for _, q := range u.RadarrQ.List {
		if q.Status == "Completed" {
			u.HandleCompleted(q.Title, "Radarr")
		} else {
			u.DeLogf("Radarr: %v (%d%%): %v", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title)
		}
	}
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (u *UnpackerPoller) HandleCompleted(name, app string) {
	d := u.getXfer(name)
	if d.Name == "" {
		u.DeLogf("%v: Transfer not found in Deluge: %v (Deluge may be unresponsive?)", app, name)
		return
	}
	path := filepath.Join(d.SavePath, d.Name)
	files := findRarFiles(path)
	if d.IsFinished && u.GetStatus(name).Status == MISSING {
		if len(files) > 0 {
			log.Printf("%v: Found %v extractable item(s) in Deluge: %v ", app, len(files), name)
			u.CreateStatus(name, path, app, files)
			go u.extractFiles(name, path, files)
		} else {
			u.DeLogf("%v: Completed Item still in Queue: %v (no extractable files found)", app, name)
		}
	}
}
