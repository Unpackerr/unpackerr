package unpacker

import (
	"log"
	"strings"
	"sync"
	"time"

	"golift.io/xtractr"
)

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	u.DeLogf("Starting Cleanup Routine (interval: 1 minute)")

	poller := time.NewTicker(u.Interval.Duration)
	cleaner := time.NewTicker(time.Minute)

	// Run all pollers once at startup.
	u.pollAllApps()

	// one go routine to rule them all.
	for {
		select {
		case <-cleaner.C:
			// Check for state changes and act on them.
			u.checkExtractDone()
			u.checkSonarrQueue()
			u.checkRadarrQueue()
			u.checkLidarrQueue()
			u.checkFolderStats()
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.folders.processEvent(event)
		case update := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.processFolderUpdate(update)
		case <-poller.C:
			// polling interval. pull API data from all apps.
			u.pollAllApps()
		case update := <-u.updates:
			// xtractr callback for app download extraction.
			u.updateQueueStatus(update)
		}
	}
}

// pollAllApps polls Sonarr and Radarr. At the same time.
func (u *Unpackerr) pollAllApps() {
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

// checkFolderStats runs at an interval to see if any folders need work done on them.
func (u *Unpackerr) checkFolderStats() {
	for name, folder := range u.folders.Folders {
		switch {
		case time.Since(folder.last) > time.Minute && folder.step == EXTRACTFAILED:
			u.folders.Folders[name].last = time.Now()
			u.folders.Folders[name].step = DOWNLOADING

			log.Printf("[Folder] Re-starting Failed Extraction: %s", folder.cnfg.Path)
		case time.Since(folder.last) > folder.cnfg.DeleteAfter.Duration && folder.step == EXTRACTED:
			u.updateQueueStatus(&Extracts{Path: name, Status: DELETED})
			delete(u.folders.Folders, name)

			if !folder.cnfg.MoveBack {
				DeleteFiles(folder.cnfg.Path + suffix)
			}

			if folder.cnfg.DeleteOrig {
				DeleteFiles(folder.cnfg.Path)
			}
		case time.Since(folder.last) > time.Minute && folder.step == DOWNLOADING:
			// update status.
			_ = u.folders.Watcher.Remove(name)
			u.folders.Folders[name].last = time.Now()
			u.folders.Folders[name].step = QUEUED
			// create a queue counter in the main history.
			u.updateQueueStatus(&Extracts{Path: name, Status: QUEUED})

			// extract it.
			queueSize, err := u.Extract(&xtractr.Xtract{
				Name:       folder.cnfg.Path,
				SearchPath: folder.cnfg.Path,
				TempFolder: !folder.cnfg.MoveBack,
				DeleteOrig: false,
				CBFunction: u.folders.xtractCallback,
			})
			if err != nil {
				log.Println("[ERROR]", err)
				return
			}

			log.Printf("[Folder] Queued: %s, queue size: %d", folder.cnfg.Path, queueSize)
		}
	}
}
