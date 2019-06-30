package unpackerpoller

import (
	"log"
	"path/filepath"
	"time"

	"github.com/golift/deluge"
	"github.com/golift/starr"
)

// PollDeluge at an interval and save the transfer list to r.Deluge
func (r *RunningData) PollDeluge(d *deluge.Deluge) error {
	var err error
	r.delS.Lock()
	defer r.delS.Unlock()
	if r.Deluge, err = d.GetXfersCompat(); err != nil {
		log.Println("Deluge Error:", err)
		return err
	}
	log.Println("Deluge Updated:", len(r.Deluge), "Transfers")
	return nil
}

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (r *RunningData) PollSonarr(s *starr.Config) {
	var err error
	r.sonS.Lock()
	defer r.sonS.Unlock()
	if r.SonarrQ, err = s.SonarrQueue(); err != nil {
		log.Println("Sonarr Error:", err)
	} else {
		log.Println("Sonarr Updated:", len(r.SonarrQ), "Items Queued")
	}
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (r *RunningData) PollRadarr(s *starr.Config) {
	var err error
	r.radS.Lock()
	defer r.radS.Unlock()
	if r.RadarrQ, err = s.RadarrQueue(); err != nil {
		log.Println("Radarr Error:", err)
	} else {
		log.Println("Radarr Updated:", len(r.RadarrQ), "Items Queued")
	}
}

// PollChange runs other tasks.
// Those tasks: a) look for things to extract, b) look for things to delete.
func (r *RunningData) PollChange() {
	// Don't start this for 2 whole minutes.
	time.Sleep(time.Minute)
	log.Println("Starting Cleanup Routine (interval: 1 minute)")
	// This runs more often because of the cleanup tasks.
	// It doesn't poll external data, unless it finds something to extract.
	ticker := time.NewTicker(time.Minute).C
	for range ticker {
		if r.Deluge == nil {
			continue // No data.
		}
		r.CheckExtractDone()
		if r.SonarrQ != nil {
			r.CheckSonarrQueue()
		}
		if r.RadarrQ != nil {
			r.CheckRadarrQueue()
		}
	}
}

// CheckExtractDone checks if an extracted item has been imported.
func (r *RunningData) CheckExtractDone() {
	log.Printf("Extract Statuses: %d actively extracting, %d queued, "+
		"%d extracted, %d imported, %d failed, %d deleted",
		r.eCount().extracting, r.eCount().queued, r.eCount().extracted,
		r.eCount().imported, r.eCount().failed, r.eCount().deleted)
	for name, data := range r.GetHistory() {
		DeLogf("Extract Status: %v (status: %v, elapsed: %v)", name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))
		switch elapsed := time.Since(r.GetStatus(name).Updated); {
		case data.Status >= DELETED && elapsed >= r.DeleteDelay*2:
			// Remove the item from history some time after it's deleted.
			log.Printf("%v: Removing History: %v", data.App, name)
			r.DeleteStatus(name)
		case data.Status < EXTRACTED || data.Status > IMPORTED:
			// Only process items that have finished extraction and are not deleted.
			continue
		case data.App == "Sonarr":
			if q := r.getSonarQitem(name); q.Status == "" {
				r.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed)
			} else {
				DeLogf("Sonarr Item Waiting For Import: %v -> %v", name, q.Status)
			}
		case data.App == "Radarr":
			if q := r.getRadarQitem(name); q.Status == "" {
				r.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed)
			} else {
				DeLogf("Radarr Item Waiting For Import: %v -> %v", name, q.Status)
			}
		}
	}
}

// HandleExtractDone checks if files should be deleted.
func (r *RunningData) HandleExtractDone(app, name string, status ExtractStatus, files []string, elapsed time.Duration) {
	switch {
	case status != IMPORTED:
		log.Printf("%v Imported: %v (delete in %v)", app, name, r.DeleteDelay)
		r.UpdateStatus(name, IMPORTED, nil)
	case elapsed >= r.DeleteDelay:
		go func() {
			status := DELETED
			if err := deleteFiles(files); err != nil {
				status = DELETEFAILED
			}
			r.UpdateStatus(name, status, nil)
		}()
	default:
		DeLogf("%v: Awaiting Delete Delay (%v remains): %v", app, r.DeleteDelay-elapsed.Round(time.Second), name)
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (r *RunningData) CheckSonarrQueue() {
	r.sonS.RLock()
	defer r.sonS.RUnlock()
	for _, q := range r.SonarrQ {
		if q.Status == "Completed" {
			r.HandleCompleted(q.Title, "Sonarr")
		} else {
			DeLogf("Sonarr: %v (%d%%): %v (Ep: %v)", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
		}
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (r *RunningData) CheckRadarrQueue() {
	r.radS.RLock()
	defer r.radS.RUnlock()
	for _, q := range r.RadarrQ {
		if q.Status == "Completed" {
			r.HandleCompleted(q.Title, "Radarr")
		} else {
			DeLogf("Radarr: %v (%d%%): %v", q.Status, int(100-(q.Sizeleft/q.Size*100)), q.Title)
		}
	}
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (r *RunningData) HandleCompleted(name, app string) {
	d := r.getXfer(name)
	if d.Name == "" {
		DeLogf("%v: Transfer not found in Deluge: %v (Deluge may be unresponsive?)", app, name)
		return
	}
	path := filepath.Join(d.SavePath, d.Name)
	files := findRarFiles(path)
	if d.IsFinished && r.GetStatus(name).Status == MISSING {
		if len(files) > 0 {
			log.Printf("%v: Found %v extractable item(s) in Deluge: %v ", app, len(files), name)
			r.CreateStatus(name, path, app, files)
			go r.extractFiles(name, path, files)
		} else {
			DeLogf("%v: Completed Item still in Queue: %v (no extractable files found)", app, name)
		}
	}
}
