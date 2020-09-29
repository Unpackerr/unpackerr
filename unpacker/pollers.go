package unpacker

import (
	"strings"
	"sync"
	"time"
)

// Run starts the loop that does the work.
func (u *Unpackerr) Run() {
	var (
		poller  = time.NewTicker(u.Interval.Duration) // poll apps at configured interval.
		cleaner = time.NewTicker(minimumInterval)     // clean at the minimum interval.
		logger  = time.NewTicker(time.Minute)         // log queue states every minute.
	)

	// Get in app queues on startup; check if items finished download & need extraction.
	u.processAppQueues()

	// one go routine to rule them all.
	for {
		select {
		case <-cleaner.C:
			// Check for state changes and act on them.
			u.checkExtractDone()
			u.checkFolderStats()
		case <-poller.C:
			// polling interval. pull API data from all apps.
			u.processAppQueues()
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
		case <-logger.C:
			// Log/print current queue counts once in a while.
			u.logCurrentQueue()
		}
	}
}

// processAppQueues polls Sonarr, Lidarr and Radarr. At the same time.
// The calls the check methods to scan their queues for changes.
func (u *Unpackerr) processAppQueues() {
	var wg sync.WaitGroup

	// Run each method in a go routine as a waitgroup.
	for _, f := range []func(){
		u.getSonarrQueue,
		u.getRadarrQueue,
		u.getLidarrQueue,
		u.getReadarrQueue,
	} {
		wg.Add(1)

		go func(f func()) {
			f()
			wg.Done()
		}(f)
	}

	wg.Wait()
	// These are not thread safe because they call handleCompletedDownload.
	u.checkSonarrQueue()
	u.checkRadarrQueue()
	u.checkLidarrQueue()
	u.checkReadarrQueue()
}

// checkExtractDone checks if an extracted item imported items needs to be deleted.
// Or if an extraction failed and needs to be restarted.
func (u *Unpackerr) checkExtractDone() {
	for name, data := range u.Map {
		switch elapsed := time.Since(data.Updated); {
		case data.App != "" && data.Status == EXTRACTFAILED && elapsed >= u.RetryDelay.Duration:
			u.Restarted++
			delete(u.Map, name)
			u.Logf("[%s] Extract failed %v ago, removed history so it can be restarted: %v",
				data.App, elapsed.Round(time.Second), name)
		case data.Status == DELETED && elapsed >= u.DeleteDelay.Duration*2:
			// Remove the item from history some time after it's deleted.
			u.Finished++
			delete(u.Map, name)
			u.Logf("[%s] Finished, Removed History: %v", data.App, name)
		}
	}
}

// checkImportsDone checks if extracted items have been imported.
func (u *Unpackerr) checkImportsDone() {
	for name, data := range u.Map {
		var inQueue bool

		switch {
		case data.App == "" || strings.HasPrefix(data.App, "Folder"):
			continue // don't handle folders here.
		case data.Status > IMPORTED:
			continue
		case strings.HasPrefix(data.App, "Sonarr"):
			inQueue = u.haveSonarrQitem(name)
		case strings.HasPrefix(data.App, "Radarr"):
			inQueue = u.haveRadarrQitem(name)
		case strings.HasPrefix(data.App, "Lidarr"):
			inQueue = u.haveLidarrQitem(name)
		case strings.HasPrefix(data.App, "Readarr"):
			inQueue = u.haveReadarrQitem(name)
		}

		if !inQueue { // We only want finished items.
			u.handleFinishedImport(data, name)
		} else if data.Status == IMPORTED && inQueue {
			// The item fell out of the app queue and came back. Reset it.
			u.Logf("%s: Resetting: %s - De-queued and returned", data.App, name)
			data.Status = WAITING
			data.Updated = time.Now()
		}

		u.Debug("%s: Status: %s (%v, elapsed: %v, found: %v)", data.App, name, data.Status.String(),
			time.Since(data.Updated).Round(time.Second), inQueue)
	}
}

// checkFolderStats runs at an interval to see if any folders need work done on them.
func (u *Unpackerr) checkFolderStats() {
	for name, folder := range u.folders.Folders {
		switch elapsed := time.Since(folder.last); {
		case EXTRACTFAILED == folder.step && elapsed >= u.RetryDelay.Duration:
			u.Logf("[Folder] Re-starting Failed Extraction: %s (failed %v ago)",
				folder.cnfg.Path, elapsed.Round(time.Second))

			folder.last = time.Now()
			folder.step = WAITING
			u.Restarted++
		case EXTRACTED == folder.step && elapsed >= folder.cnfg.DeleteAfter.Duration:
			// Folder reached delete delay (after extraction), nuke it.
			u.updateQueueStatus(&Extracts{Path: name, Status: DELETED})
			delete(u.folders.Folders, name)

			// Only delete the extracted files if DeleteAfter is greater than 0.
			if !folder.cnfg.MoveBack && folder.cnfg.DeleteAfter.Duration > 0 {
				u.DeleteFiles(strings.TrimRight(name, `/\`) + suffix)
			}

			if folder.cnfg.DeleteOrig {
				u.DeleteFiles(name)
			}
		case WAITING == folder.step && elapsed >= u.StartDelay.Duration:
			// The folder hasn't been written to in a while, extract it.
			u.extractFolder(name, folder)
		}
	}
}
