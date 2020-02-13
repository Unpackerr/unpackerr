package unpacker

import (
	"fmt"
	"log"
	"path/filepath"
	"time"
)

// torrent is what we care about. no usenet..
const torrent = "torrent"

// PollSonarr saves the Sonarr Queue to r.SonarrQ
func (u *Unpackerr) PollSonarr(sonarr *sonarrConfig) error {
	var err error

	sonarr.Lock()
	defer sonarr.Unlock()

	if sonarr.List, err = sonarr.SonarrQueue(); err != nil {
		return err
	}

	log.Printf("Sonarr Updated (%s): %d Items Queued", sonarr.URL, len(sonarr.List))

	return nil
}

// PollRadarr saves the Radarr Queue to r.RadarrQ
func (u *Unpackerr) PollRadarr(radarr *radarrConfig) error {
	var err error

	radarr.Lock()
	defer radarr.Unlock()

	if radarr.List, err = radarr.RadarrQueue(); err != nil {
		return err
	}

	log.Printf("Radarr Updated (%s): %d Items Queued", radarr.URL, len(radarr.List))

	return nil
}

// CheckExtractDone checks if an extracted item has been imported.
func (u *Unpackerr) CheckExtractDone() {
	u.History.RLock()

	defer func() {
		u.History.RUnlock()
		e := u.eCount()
		log.Printf("Extract Statuses: %d extracting, %d queued, %d extracted, "+
			"%d imported, %d failed, %d deleted. Restarted: %d, Finished: %d",
			e.extracting, e.queued, e.extracted, e.imported, e.failed, e.deleted,
			u.Restarted, u.Finished)
	}()

	for name, data := range u.History.Map {
		u.DeLogf("Extract Status: %v (status: %v, elapsed: %v)", name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))

		switch {
		case data.Status == EXTRACTFAILED:
			go u.retryFailedExtract(data, name)
		case data.Status >= DELETED && time.Since(data.Updated) >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			go u.finishFinished(data.App, name)
		case data.Status < EXTRACTED || data.Status > IMPORTED:
			continue // Only process items that have finished extraction and are not deleted.
		case data.App == "Sonarr":
			go u.handleSonarr(data, name)
		case data.App == "Radarr":
			go u.handleRadarr(data, name)
		}
	}
}

func (u *Unpackerr) retryFailedExtract(data Extracts, name string) {
	// Only retry after retry time expires.
	if time.Since(data.Updated) < u.RetryDelay.Duration {
		return
	}

	u.History.Lock()
	defer u.History.Unlock()
	u.History.Restarted++

	log.Printf("%v: Extract failed, removing history so it can be restarted: %v", data.App, name)
	delete(u.History.Map, name)
}

func (u *Unpackerr) finishFinished(app, name string) {
	u.History.Lock()
	defer u.History.Unlock()
	u.History.Finished++

	log.Printf("%v: Finished, Removing History: %v", app, name)
	delete(u.History.Map, name)
}

func (u *Unpackerr) handleRadarr(data Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()

	if item := u.getRadarQitem(name); item.Status != "" {
		u.DeLogf("%s Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
		return // We only want finished items.
	} else if item.Protocol != torrent && item.Protocol != "" {
		return // We only want torrents.
	}

	if s := u.HandleExtractDone(data, name); s != data.Status {
		// Status changed.
		data.Status, data.Updated = s, time.Now()
		u.History.Map[name] = data
	}
}

func (u *Unpackerr) handleSonarr(data Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()

	if item := u.getSonarQitem(name); item.Status != "" {
		u.DeLogf("%s Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
		return // We only want finished items.
	} else if item.Protocol != torrent && item.Protocol != "" {
		return // We only want torrents.
	}

	if s := u.HandleExtractDone(data, name); s != data.Status {
		data.Status, data.Updated = s, time.Now()
		u.History.Map[name] = data
	}
}

// HandleExtractDone checks if files should be deleted.
func (u *Unpackerr) HandleExtractDone(data Extracts, name string) ExtractStatus {
	switch elapsed := time.Since(data.Updated); {
	case data.Status != IMPORTED:
		log.Printf("%v Imported: %v (delete in %v)", data.App, name, u.DeleteDelay)
		return IMPORTED
	case elapsed >= u.DeleteDelay.Duration:
		deleteFiles(data.Files)
		return DELETED
	default:
		u.DeLogf("%v: Awaiting Delete Delay (%v remains): %v",
			data.App, u.DeleteDelay.Duration-elapsed.Round(time.Second), name)
		return data.Status
	}
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) CheckSonarrQueue() {
	check := func(sonarr *sonarrConfig) {
		sonarr.RLock()
		defer sonarr.RUnlock()

		for _, q := range sonarr.List {
			if q.Status == "Completed" && q.Protocol == torrent {
				name := fmt.Sprintf("Sonarr (%s)", sonarr.URL)
				go u.HandleCompleted(q.Title, name, sonarr.Path)
			} else {
				u.DeLogf("Sonarr (%s): %s (%s:%d%%): %v (Ep: %v)",
					sonarr.URL, q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
			}
		}
	}

	for _, sonarr := range u.Sonarr {
		check(sonarr)
	}
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) CheckRadarrQueue() {
	check := func(radarr *radarrConfig) {
		radarr.RLock()
		defer radarr.RUnlock()

		for _, q := range radarr.List {
			if q.Status == "Completed" && q.Protocol == torrent {
				name := fmt.Sprintf("Radarr (%s)", radarr.URL)
				go u.HandleCompleted(q.Title, name, radarr.Path)
			} else {
				u.DeLogf("Radarr (%s): %s (%s:%d%%): %v",
					radarr.URL, q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title)
			}
		}
	}

	for _, radarr := range u.Radarr {
		check(radarr)
	}
}

func (u *Unpackerr) historyExists(name string) (ok bool) {
	u.History.RLock()
	defer u.History.RUnlock()
	_, ok = u.History.Map[name]

	return
}

// HandleCompleted checks if a completed sonarr or radarr item needs to be extracted.
func (u *Unpackerr) HandleCompleted(name, app, path string) {
	path = filepath.Join(path, name)
	files := FindRarFiles(path)

	if !u.historyExists(name) {
		if len(files) > 0 {
			log.Printf("%s: Found %d extractable item(s): %s (%s)", app, len(files), name, path)
			u.CreateStatus(name, path, app, files)
			u.extractFiles(name, path, files)
		} else {
			u.DeLogf("%s: Completed item still in queue: %s, no extractable files found at: %s", app, name, path)
		}
	}
}
