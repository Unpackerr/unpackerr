package unpacker

import (
	"log"
	"strings"
	"sync"
	"time"
)

// PollAllApps Polls  Sonarr and Radarr. At the same time.
func (u *Unpackerr) PollAllApps() {
	var wg sync.WaitGroup

	for _, sonarr := range u.Sonarr {
		if sonarr.APIKey == "" {
			u.DeLogf("Sonarr (%s): skipped, no API key", sonarr.URL)
			continue
		}

		wg.Add(1)

		go func(sonarr *sonarrConfig) {
			if err := u.PollSonarr(sonarr); err != nil {
				log.Printf("[ERROR] Sonarr (%s): %v", sonarr.URL, err)
			}

			wg.Done()
		}(sonarr)
	}

	for _, radarr := range u.Radarr {
		if radarr.APIKey == "" {
			u.DeLogf("Radarr (%s): skipped, no API key", radarr.URL)
			continue
		}

		wg.Add(1)

		go func(radarr *radarrConfig) {
			if err := u.PollRadarr(radarr); err != nil {
				log.Printf("[ERROR] Radarr (%s): %v", radarr.URL, err)
			}

			wg.Done()
		}(radarr)
	}

	for _, lidarr := range u.Lidarr {
		if lidarr.APIKey == "" {
			u.DeLogf("Lidarr (%s): skipped, no API key", lidarr.URL)
			continue
		}

		wg.Add(1)

		go func(lidarr *lidarrConfig) {
			if err := u.PollLidarr(lidarr); err != nil {
				log.Printf("[ERROR] Lidarr (%s): %v", lidarr.URL, err)
			}

			wg.Done()
		}(lidarr)
	}

	wg.Wait()
}

// CheckExtractDone checks if an extracted item has been imported.
func (u *Unpackerr) CheckExtractDone() {
	u.History.RLock()

	defer func() {
		u.History.RUnlock()
		e := u.eCount()
		log.Printf("Queue: [%d queued] [%d extracting] [%d extracted] [%d imported]"+
			" [%d failed] [%d deleted], Totals: [%d restarted] [%d finished]",
			e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
			u.Restarted, u.Finished)
	}()

	for name, data := range u.History.Map {
		u.DeLogf("%s: Extract Status: %v (status: %v, elapsed: %v)", data.App, name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))

		switch {
		case data.Status == EXTRACTFAILED:
			go u.retryFailedExtract(data, name)
		case data.Status >= DELETED && time.Since(data.Updated) >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			go u.finishFinished(data.App, name)
		case data.Status < EXTRACTED || data.Status > IMPORTED:
			continue // Only process items that have finished extraction and are not deleted.
		case strings.HasPrefix(data.App, "Sonarr"):
			go u.handleSonarr(data, name)
		case strings.HasPrefix(data.App, "Radarr"):
			go u.handleRadarr(data, name)
		case strings.HasPrefix(data.App, "Lidarr"):
			go u.handleLidarr(data, name)
		case strings.HasPrefix(data.App, "Folder"):
			go u.handleFolder(data, name)
		}
	}
}

func (u *Unpackerr) retryFailedExtract(data *Extracts, name string) {
	// Only retry after retry time expires.
	if time.Since(data.Updated) < u.RetryDelay.Duration {
		return
	}

	log.Printf("%v: Extract failed %v ago, removing history so it can be restarted: %v",
		data.App, time.Since(data.Updated), name)

	if data.App == "Folder" {
		u.folders.Updates <- &update{Step: QUEUED, Name: name}
	}

	u.History.Lock()
	defer u.History.Unlock()
	u.History.Restarted++
	delete(u.History.Map, name)
}

func (u *Unpackerr) finishFinished(app, name string) {
	u.History.Lock()
	defer u.History.Unlock()
	u.History.Finished++
	delete(u.History.Map, name)
	log.Printf("[%v] Finished, Removed History: %v", app, name)
}

// HandleExtractDone checks if files should be deleted.
func (u *Unpackerr) HandleExtractDone(data *Extracts, name string) ExtractStatus {
	switch elapsed := time.Since(data.Updated); {
	case data.Status != IMPORTED:
		log.Printf("[%v] Imported: %v (delete in %v)", data.App, name, u.DeleteDelay)
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

// HandleCompleted checks if a completed item needs to be extracted.
func (u *Unpackerr) HandleCompleted(name, app, path string) {
	if files := FindRarFiles(path); len(files) > 0 {
		log.Printf("%s: Found %d extractable item(s): %s (%s)", app, len(files), name, path)
		u.CreateStatus(name, path, app, files)
		go u.extractFiles(name, path, files, true)

		return
	}

	u.DeLogf("%s: Completed item still in queue: %s, no extractable files found at: %s", app, name, path)
}
