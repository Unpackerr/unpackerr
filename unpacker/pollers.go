package unpacker

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	u.DeLogf("Starting Cleanup Routine (interval: 1 minute)")

	poller := time.NewTicker(u.Interval.Duration)
	cleaner := time.NewTicker(time.Minute)

	u.PollAllApps() // Run all pollers once at startup.

	for { // one go routine.
		select {
		case <-cleaner.C:
			u.checkExtractDone()
			u.checkSonarrQueue()
			u.checkRadarrQueue()
			u.checkLidarrQueue()
		case <-poller.C:
			u.PollAllApps()
		case update := <-u.updates:
			u.updateStatus(update)
		}
	}
}

// PollAllApps Polls  Sonarr and Radarr. At the same time.
func (u *Unpackerr) PollAllApps() {
	const threeItems = 3

	var wg *sync.WaitGroup

	wg.Add(threeItems)

	go func() {
		u.PollSonarr()
		wg.Done()
	}()

	go func() {
		u.PollRadarr()
		wg.Done()
	}()

	go func() {
		u.PollLidarr()
		wg.Done()
	}()

	wg.Wait()
}

// LidarrQueuePageSize is how many items we request from Lidarr.
// If you have more than this many items queued.. oof.
const LidarrQueuePageSize = 2000

// PollLidarr saves the Lidarr Queue
func (u *Unpackerr) PollLidarr() {
	var err error

	for _, server := range u.Lidarr {
		if server.APIKey == "" {
			u.DeLogf("Lidarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.LidarrQueue(LidarrQueuePageSize); err != nil {
			log.Printf("[ERROR] Lidarr (%s): %v", server.URL, err)
			return
		}

		log.Printf("[Lidarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// PollSonarr saves the Sonarr Queue
func (u *Unpackerr) PollSonarr() {
	var err error

	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.DeLogf("Sonarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.SonarrQueue(); err != nil {
			log.Printf("[ERROR] Sonarr (%s): %v", server.URL, err)
			return
		}

		log.Printf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// PollRadarr saves the Radarr Queue
func (u *Unpackerr) PollRadarr() {
	var err error

	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.DeLogf("Radarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.RadarrQueue(); err != nil {
			log.Printf("[ERROR] Radarr (%s): %v", server.URL, err)
		}

		log.Printf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// checkExtractDone checks if an extracted item has been imported.
// Or an imported items needs to be deleted.
// Or if an extraction failed and needs to be restarted.
// Runs every minute by the cleanup routine.
func (u *Unpackerr) checkExtractDone() {
	e := &eCounters{}
	defer log.Printf("Queue: [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted], Totals: [%d restarted] [%d finished]",
		e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
		u.Restarted, u.Finished)

	for name, data := range u.Map {
		u.DeLogf("%s: Status: %v (status: %v, elapsed: %v)", data.App, name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))

		switch {
		case data.Status == EXTRACTFAILED && time.Since(data.Updated) < u.RetryDelay.Duration:
			u.Restarted++
			delete(u.Map, name)
			log.Printf("[%s] Extract failed %v ago, removed history so it can be restarted: %v",
				data.App, time.Since(data.Updated), name)
		case data.Status == DELETED && time.Since(data.Updated) >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			delete(u.Map, name)
			log.Printf("[%v] Finished, Removed History: %v", data.App, name)
		case data.Status != EXTRACTED && data.Status != IMPORTED:
			break // Only process items that have finished extraction and are not deleted.
		case strings.HasPrefix(data.App, "Sonarr"):
			if item := u.getSonarQitem(name); item != nil {
				u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
				break // We only want finished items.
			}

			u.handleFinishedImport(data, name)
		case strings.HasPrefix(data.App, "Radarr"):
			if item := u.getRadarQitem(name); item != nil {
				u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
				break // We only want finished items.
			}

			u.handleFinishedImport(data, name)
		case strings.HasPrefix(data.App, "Lidarr"):
			if item := u.getLidarQitem(name); item != nil {
				u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
				break // We only want finished items.
			}

			u.handleFinishedImport(data, name)
		}

		u.eCount(e, data.Status)
	}
}
