package unpacker

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	poller := time.NewTicker(u.Interval.Duration)
	cleaner := time.NewTicker(15 * time.Second)
	logger := time.NewTicker(u.Interval.Duration / 2)

	// Fill in all app queues once on startup.
	u.saveAllAppQueues()

	// one go routine to rule them all.
	for {
		select {
		case <-logger.C:
			// Log counts once in a while.
			e := u.eCount()
			log.Printf("Queue: [%d queued] [%d extracting] [%d extracted] [%d imported]"+
				" [%d failed] [%d deleted], Totals: [%d restarted] [%d finished]",
				e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
				u.Restarted, u.Finished)
		case <-cleaner.C:
			// Check for state changes and act on them.
			u.checkExtractDone()
			u.checkFolderStats()
		case <-poller.C:
			// polling interval. pull API data from all apps.
			u.saveAllAppQueues()
			// check if things finished downloading and need extraction.
			u.checkSonarrQueue()
			u.checkRadarrQueue()
			u.checkLidarrQueue()
			// check if things got imported and now need to be deleted.
			u.checkImportsDone()
		case update := <-u.updates:
			// xtractr callback for app download extraction.
			u.updateQueueStatus(update)
		case update := <-u.folders.Updates:
			// xtractr callback for a watched folder extraction.
			u.processFolderUpdate(update)
		case event := <-u.folders.Events:
			// file system event for watched folder.
			u.folders.processEvent(event)
		}
	}
}

// getAllAppQueues polls Sonarr and Radarr. At the same time.
func (u *Unpackerr) saveAllAppQueues() {
	const threeItems = 3

	var wg *sync.WaitGroup

	wg.Add(threeItems)

	go func() {
		u.getSonarrQueue()
		wg.Done()
	}()

	go func() {
		u.getRadarrQueue()
		wg.Done()
	}()

	go func() {
		u.getLidarrQueue()
		wg.Done()
	}()

	wg.Wait()
}

// checkExtractDone checks if an extracted item imported items needs to be deleted.
// Or if an extraction failed and needs to be restarted.
func (u *Unpackerr) checkExtractDone() {
	for name, data := range u.Map {
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
		}
	}
}

// checkImportsDone checks if extracted items have been imported.
func (u *Unpackerr) checkImportsDone() {
	for name, data := range u.Map {
		u.DeLogf("%s: Status: %v (status: %v, elapsed: %v)", data.App, name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second))

		switch {
		case strings.HasPrefix(data.App, "Sonarr"):
			if u.getSonarQitem(name) == nil {
				u.handleFinishedImport(data, name) // We only want finished items.
			}
		case strings.HasPrefix(data.App, "Radarr"):
			if u.getRadarQitem(name) == nil {
				u.handleFinishedImport(data, name) // We only want finished items.
			}
		case strings.HasPrefix(data.App, "Lidarr"):
			if u.getLidarQitem(name) == nil {
				u.handleFinishedImport(data, name) // We only want finished items.
			}
		}
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
			u.extractFolder(name, folder)
		}
	}
}
