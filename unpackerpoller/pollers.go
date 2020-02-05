package unpackerpoller

import (
	"log"
	"path/filepath"
	"time"
)

// torrent is what we care about. no usenet..
const torrent = "torrent"

// PollDeluge at an interval and save the transfer list to r.Deluge
func (u *UnpackerPoller) PollDeluge() error {
	var err error

	u.Xfers.Lock()
	defer u.Xfers.Unlock()

	if u.Xfers.Map, err = u.Deluge.GetXfersCompat(); err != nil {
		return err
	}

	log.Println("Deluge Updated:", len(u.Xfers.Map), "Transfers")

	return nil
}

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (u *UnpackerPoller) PollSonarr() error {
	var err error

	u.SonarrQ.Lock()
	defer u.SonarrQ.Unlock()

	if u.SonarrQ.List, err = u.Sonarr.SonarrQueue(); err != nil {
		return err
	}

	log.Println("Sonarr Updated:", len(u.SonarrQ.List), "Items Queued")

	return nil
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (u *UnpackerPoller) PollRadarr() error {
	var err error

	u.RadarrQ.Lock()
	defer u.RadarrQ.Unlock()

	if u.RadarrQ.List, err = u.Radarr.RadarrQueue(); err != nil {
		return err
	}

	log.Println("Radarr Updated:", len(u.RadarrQ.List), "Items Queued")

	return nil
}

// PollChange runs other tasks.
// Those tasks: a) look for things to extract, b) look for things to delete.
// This runs more often because of the cleanup tasks.
// It doesn't poll external data, unless it finds something to extract.
func (u *UnpackerPoller) PollChange() {
	u.DeLogf("Starting Cleanup Routine (interval: 1 minute)")

	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-ticker.C:
			u.CheckExtractDone()
			u.CheckSonarrQueue()
			u.CheckRadarrQueue()
		case <-u.StopChan:
			ticker.Stop()
			return
		}
	}
}

// CheckExtractDone checks if an extracted item has been imported.
func (u *UnpackerPoller) CheckExtractDone() {
	u.History.Lock()

	defer func() {
		u.History.Unlock()
		ec := u.eCount()
		log.Printf("Extract Statuses: %d extracting, %d queued, "+
			"%d extracted, %d imported, %d failed, %d deleted. Finished: %d",
			ec.extracting, ec.queued, ec.extracted,
			ec.imported, ec.failed, ec.deleted, ec.finished)
	}()

	for name, data := range u.History.Map {
		u.DeLogf("Extract Status: %v (status: %v, elapsed: %v)", name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))

		switch elapsed := time.Since(data.Updated); {
		case data.Status >= DELETED && elapsed >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			u.History.Finished++

			log.Printf("%v: Finished, Removing History: %v", data.App, name)
			delete(u.History.Map, name)
		case data.Status < EXTRACTED || data.Status > IMPORTED:
			// Only process items that have finished extraction and are not deleted.
			continue
		case data.App == "Sonarr":
			item := u.getSonarQitem(name)
			if item.Status != "" {
				u.DeLogf("Sonarr Item Waiting For Import (%s): %v -> %v", item.Protocol, name, item.Status)
				continue
			}

			if item.Protocol != torrent && item.Protocol != "" {
				continue
			}

			if s := u.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed); s != data.Status {
				data.Status = s
				data.Updated = time.Now()
				u.History.Map[name] = data
			}
		case data.App == "Radarr":
			item := u.getRadarQitem(name)
			if item.Status != "" {
				u.DeLogf("Radarr Item Waiting For Import (%s): %v -> %v", item.Protocol, name, item.Status)
				continue
			}

			if item.Protocol != torrent && item.Protocol != "" {
				continue
			}

			if s := u.HandleExtractDone(data.App, name, data.Status, data.Files, elapsed); s != data.Status {
				data.Status = s
				data.Updated = time.Now()
				u.History.Map[name] = data
			}
		}
	}
}

// HandleExtractDone checks if files should be deleted.
func (u *UnpackerPoller) HandleExtractDone(app, name string, status ExtractStatus, files []string, elapsed time.Duration) ExtractStatus {
	switch {
	case status != IMPORTED:
		log.Printf("%v Imported: %v (delete in %v)", app, name, u.DeleteDelay)
		return IMPORTED
	case elapsed >= u.DeleteDelay.Duration:
		deleteFiles(files)
		return DELETED
	default:
		u.DeLogf("%v: Awaiting Delete Delay (%v remains): %v", app, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
		return status
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *UnpackerPoller) CheckSonarrQueue() {
	u.SonarrQ.RLock()
	defer u.SonarrQ.RUnlock()

	for _, q := range u.SonarrQ.List {
		if q.Status == "Completed" && q.Protocol == torrent {
			go u.HandleCompleted(q.Title, "Sonarr")
		} else {
			u.DeLogf("Sonarr: %s (%s:%d%%): %v (Ep: %v)", q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
		}
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *UnpackerPoller) CheckRadarrQueue() {
	u.RadarrQ.RLock()
	defer u.RadarrQ.RUnlock()

	for _, q := range u.RadarrQ.List {
		if q.Status == "Completed" && q.Protocol == torrent {
			go u.HandleCompleted(q.Title, "Radarr")
		} else {
			u.DeLogf("Radarr: %s (%s:%d%%): %v", q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title)
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
	files := FindRarFiles(path)

	u.History.RLock()

	_, ok := u.History.Map[name]

	u.History.RUnlock()

	if !ok && d.IsFinished {
		if len(files) > 0 {
			log.Printf("%v: Found %v extractable item(s) in Deluge: %v ", app, len(files), name)
			u.CreateStatus(name, path, app, files)
			u.extractFiles(name, path, files)
		} else {
			u.DeLogf("%v: Completed Item still in Queue: %v (no extractable files found)", app, name)
		}
	}
}
