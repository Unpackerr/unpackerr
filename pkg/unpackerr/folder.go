package unpackerr

/* Folder extract / callback / cleanup — the watch tracker lives in pkg/folders. */

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/Unpackerr/unpackerr/pkg/folders"
	"golift.io/xtractr"
)

func (u *Unpackerr) validateFolders() error {
	return validateFolderList(u.Folders)
}

func validateFolderList(list []*FolderConfig) error {
	if err := folders.ValidateList(list, parseOptionalMaxBytes); err != nil {
		return fmt.Errorf("validating folders: %w", err)
	}

	return nil
}

// PollFolders begins the routines to watch folders for changes.
// if those changes include the addition of compressed files, they
// are processed for exctraction.
func (u *Unpackerr) PollFolders() {
	var (
		flist []string
		err   error
	)

	if isRunningInDocker() && u.Folder.Interval.Duration == 0 {
		u.Folder.Interval.Duration = folders.DefaultPollInterval
	}

	// Abs-expand a clone so GET /api/config/folders/live keeps the configured
	// path (file vs env), not the runtime filepath.Abs rewrite.
	watched, flist := folders.Check(folders.CloneList(u.Folders), u.Logger)

	tracker, err := u.Folder.NewWatcher(watched, u.Logger, updateChanBuf, suffix)
	if err != nil {
		u.Errorf("Watching Folders: %s", err)
		return
	}

	u.folders = tracker
	// do not close either watcher.

	if len(watched) == 0 {
		return
	}

	go u.folders.WatchFSNotify()

	u.Printf("[Folder] Watching (fsnotify): %s", strings.Join(flist, ", "))

	// Setting an interval of any value less than 5 milliseconds
	// (except zero in docker) allows disabling the poller.
	if u.Folder.Interval.Duration < folders.MinimumPollInterval {
		return
	}

	go func() {
		if err := u.folders.StartPoller(u.Folder.Interval.Duration); err != nil {
			u.Errorf("%s", err)
		}
	}()

	u.Printf("[Folder] Polling @ %s: %s", u.Folder.Interval.String(), strings.Join(flist, ", "))
}

// extractTrackedItem starts an archive or folder's extraction after it hasn't been written to in a while.
func (u *Unpackerr) extractTrackedItem(name string, folder *Folder, now time.Time) {
	u.folders.Remove(name) // stop the fs watcher(s).
	// update status.
	u.folders.Folders[name].Updated = now
	u.folders.Folders[name].Status = QUEUED

	if u.skipR00Extraction(name, now) {
		return
	}

	// create a queue counter in the main history; add to u.Map and send webhook for a new folder.
	u.lockHistory()
	item := u.updateQueueStatus(&newStatus{Name: name, Status: QUEUED}, u.folders.Folders[name].Updated, true)
	u.unlockHistory()
	u.updateHistory(FolderString + ": " + name)

	exclude := folderExcludeSuffixes(name, folder.Config)

	if folder.Config.MoveBack {
		found := xtractr.FindCompressedFiles(xtractr.Filter{
			Path:          name,
			ExcludeSuffix: exclude,
			AllowSymlinks: folder.Config.AllowSymlinks,
		})

		snap, err := keepDirSnapshot(folder.PreFiles, archiveSnapshotPaths(name, found)...)
		if err != nil {
			u.Errorf("[Folder] Snapshot dests for remnant check: %v", err)
		} else {
			folder.PreFiles = snap
		}
	}

	// extract it.
	queueSize, err := u.Extract(&xtractr.Xtract{
		Password:         u.getPasswordFromPath(name),
		Passwords:        u.Passwords,
		Name:             name,
		Path:             name,
		ExcludeSuffix:    exclude,
		AllowSymlinks:    folder.Config.AllowSymlinks,
		MaxBytes:         folder.Config.ResolvedMaxBytes,
		MaxFiles:         folder.Config.MaxFiles,
		MaxRatio:         folder.Config.MaxRatio,
		MaxNested:        folder.Config.MaxNested,
		ExtrasMaxDepth:   folder.Config.ExtrasMaxDepth,
		TempFolder:       !folder.Config.MoveBack,
		ExtractTo:        folder.Config.ExtractPath,
		DeleteOrig:       false,
		CBChannel:        u.folders.Updates,
		CBFunction:       nil,
		Progress:         u.progressUpdateCallback(item),
		LogFile:          !folder.Config.DisableLog,
		DisableRecursion: folder.Config.DisableRecursion,
	})
	if err != nil {
		u.Errorf("[ERROR] %v", err)
		return
	}

	u.Printf("[Folder] Queued: %s, queue size: %d", name, queueSize)
}

// skipR00Extraction drops a tracked .r00 when a sibling .rar exists (xtractr would extract the rar).
func (u *Unpackerr) skipR00Extraction(name string, now time.Time) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".r00") ||
		!xtractr.CheckR00ForRarFile(getFileList(filepath.Dir(name)), filepath.Base(name)) {
		return false
	}

	u.Printf("[Folder] Removing tracked item without extraction: %v (rar file exists)", name)
	u.folders.Folders[name].Status = EXTRACTEDNOTHING
	u.lockHistory()
	u.updateQueueStatus(&newStatus{Name: name, Status: EXTRACTEDNOTHING}, now, false)
	u.unlockHistory()

	return true
}

// folderExcludeSuffixes returns archive suffixes to ignore when scanning for items to extract.
// For watched archive files with disable_recursion enabled, exclude all archive suffixes so
// extracted nested archives are not picked up by follow-up scans in the extraction library.
func folderExcludeSuffixes(path string, cfg *FolderConfig) []string {
	exclude := []string{}
	if !cfg.ExtractISOs {
		exclude = append(exclude, ".iso")
	}

	if !cfg.DisableRecursion {
		return exclude
	}

	stat, err := os.Stat(path)
	if err != nil || stat.IsDir() || !xtractr.IsArchiveFile(path) {
		return exclude
	}

	return append(exclude, xtractr.SupportedExtensions()...)
}

func getFileList(path string) []os.FileInfo {
	dir, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer dir.Close()

	if stat, err := dir.Stat(); err != nil || !stat.IsDir() {
		return nil
	}

	fileList, err := dir.Readdir(-1)
	if err != nil {
		return nil
	}

	return fileList
}

// folderXtractrCallback is run twice by the xtractr library when the extraction begins, and finishes.
func (u *Unpackerr) folderXtractrCallback(resp *xtractr.Response) {
	now := resp.Started.Add(resp.Elapsed)

	u.lockHistory()

	folder, found := u.folders.Folders[resp.X.Name]
	item := u.Map[resp.X.Name]

	if !found || item == nil {
		delete(u.folders.Folders, resp.X.Name)
		delete(u.Map, resp.X.Name)
		u.unlockHistory()

		return
	}

	if !resp.Done {
		item.XProg.Archives = resp.Archives.Count() + resp.Extras.Count()
		folder.Status = EXTRACTING
		u.Printf("[Folder] Extraction Started: %s, retries: %d, items in queue: %d", resp.X.Name, folder.Retries, resp.Queued)
		folder.Updated = now
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.Status}, folder.Updated, true)
		u.unlockHistory()

		return
	}

	if errors.Is(resp.Error, xtractr.ErrNoCompressedFiles) {
		folder.Status = EXTRACTEDNOTHING
		u.Printf("[Folder] %s: %s: %v", folder.Status.Desc(), resp.X.Name, resp.Error)
		folder.Updated = now
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.Status}, folder.Updated, true)
		u.unlockHistory()

		return
	}

	preFiles := folder.PreFiles
	retries := folder.Retries
	configPath := folder.Config.Path

	u.unlockHistory()

	remnantStatus, remnants := u.finishFolderExtractWork(resp, preFiles, retries, configPath)

	u.lockHistory()
	defer u.unlockHistory()

	folder, found = u.folders.Folders[resp.X.Name]
	if !found {
		return
	}

	u.commitFolderExtract(folder, resp, remnantStatus, remnants)
	folder.Updated = now
	u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.Status}, folder.Updated, true)
}

// finishFolderExtractWork logs, records metrics, and classifies remnants without
// holding History.mu — remnant cleanup may RemoveAll large trees.
func (u *Unpackerr) finishFolderExtractWork(
	resp *xtractr.Response,
	preFiles map[string]os.FileInfo,
	retries uint,
	configPath string,
) (ExtractStatus, bool) {
	if resp.Error != nil {
		u.Errorf("[Folder] %s: %s: %v", EXTRACTFAILED.Desc(), resp.X.Name, resp.Error)
	} else {
		u.Printf("[Folder] Extraction Finished: %s => elapsed: %v, archives: %d, "+
			"extra archives: %d, files extracted: %d, written: %sB",
			resp.X.Name, resp.Elapsed.Round(time.Second), resp.Archives.Count(),
			resp.Extras.Count(), len(resp.NewFiles), bytefmt.ByteSize(resp.Size))
	}

	u.updateMetrics(resp, FolderString, configPath)

	return u.handleRemnants(resp, preFiles, retries)
}

// commitFolderExtract applies remnant/error/success status under History.mu.
// remnant_action=off sets noRetry so checkFolderStats will not restart.
func (u *Unpackerr) commitFolderExtract(folder *Folder, resp *xtractr.Response, status ExtractStatus, remnants bool) {
	if resp.Error != nil {
		folder.Archives = resp.Archives
	}

	if remnants {
		u.finishFolderRemnants(folder, resp, status)
		return
	}

	if resp.Error != nil {
		folder.Status = EXTRACTFAILED
		return
	}

	folder.Archives = resp.Archives
	folder.Status = EXTRACTED
	folder.Files = resp.NewFiles
}

func (u *Unpackerr) finishFolderRemnants(folder *Folder, resp *xtractr.Response, status ExtractStatus) {
	if status == WAITING {
		u.Printf("[Folder] Cleared interrupted-extraction remnant(s), restarting extraction: %s", resp.X.Name)

		folder.Status = EXTRACTFAILED

		return
	}

	if remnantAction(u.RemnantAction) == "off" {
		folder.NoRetry = true
	}

	u.Errorf("[Folder] Extraction blocked by interrupted-extraction remnant(s): %s", resp.X.Name)

	folder.Status = EXTRACTFAILED
}

// processEvent is here to process the event in the `*Unpackerr` scope before sending it back to the `*Folders` scope.
func (u *Unpackerr) processEvent(event *eventData, now time.Time) {
	if event == nil || event.Config == nil {
		return
	}

	// Do not watch our own log file.
	if event.File == u.LogFile || event.File == u.Webserver.LogFile {
		return
	}

	u.folders.ProcessEvent(event, now)
	u.syncFolderQueue(filepath.Join(event.Config.Path, event.Name))
}

// syncFolderQueue mirrors a watched folder into the overview queue while it is
// still tracking writes (before start_delay queues extraction).
func (u *Unpackerr) syncFolderQueue(dirPath string) {
	u.lockHistory()
	defer u.unlockHistory()

	folder, ok := u.folders.Folders[dirPath]
	if !ok {
		if item := u.Map[dirPath]; item != nil && item.App == FolderString && item.Status == WAITING {
			delete(u.Map, dirPath)
		}

		return
	}

	if folder.Status != WAITING {
		return
	}

	if _, exists := u.Map[dirPath]; !exists {
		u.updateQueueStatus(&newStatus{Name: dirPath, Status: WAITING}, folder.Updated, false)
		return
	}

	item := u.Map[dirPath]
	if item.Status != WAITING {
		return
	}

	item.Updated = folder.Updated
}

func (u *Unpackerr) checkWaitingFolder(name string, folder *Folder, now time.Time) {
	if _, err := os.Stat(name); err != nil {
		delete(u.folders.Folders, name)
		u.folders.Remove(name)
		u.syncFolderQueue(name)

		return
	}

	if now.Sub(folder.Updated) >= u.StartDelay.Duration {
		u.extractTrackedItem(name, folder, now)
	}
}

// checkFolderStats runs at an interval to see if any folders need work done on them.
// This runs on an interval ticker in the main go routine.
func (u *Unpackerr) checkFolderStats(now time.Time) {
	for name, folder := range u.folders.Folders {
		switch elapsed := now.Sub(folder.Updated); {
		case WAITING == folder.Status:
			u.checkWaitingFolder(name, folder, now)
		case EXTRACTEDNOTHING == folder.Status:
			// Wait until this item hasn't been touched for a while, so it doesn't re-queue.
			if now.Sub(folder.Updated) > u.StartDelay.Duration {
				// Ignore "no compressed files" errors for folders.
				u.lockHistory()
				delete(u.Map, name)
				u.unlockHistory()
				delete(u.folders.Folders, name)
			}
		case EXTRACTFAILED == folder.Status && folder.NoRetry:
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, true)
			u.unlockHistory()
			delete(u.folders.Folders, name)
			u.Printf("[Folder] Remnant left in place (remnant_action=off), giving up: %s", name)
		case EXTRACTFAILED == folder.Status && elapsed >= u.RetryDelay.Duration &&
			folder.Retries < u.maxRetries():
			u.lockHistory()
			u.Retries++
			u.unlockHistory()

			folder.Retries++
			folder.Updated = now
			folder.Status = WAITING
			u.Printf("[Folder] Re-starting Failed Extraction: %s (%d/%d, failed %v ago)",
				folder.Config.Path, folder.Retries, u.maxRetries(), elapsed.Round(time.Second))
		case EXTRACTFAILED == folder.Status && folder.Retries < u.maxRetries():
			// This empty block is to avoid deleting an item that needs more retries.
		case EXTRACTFAILED == folder.Status && folder.Retries >= u.maxRetries():
			// Retries exhausted — clean up to prevent the item from staying in the map forever.
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, true)
			u.unlockHistory()
			delete(u.folders.Folders, name)
			u.Printf("[Folder] Retries exhausted (%d/%d), giving up: %s", folder.Retries, u.maxRetries(), name)
		case EXTRACTED == folder.Status && folder.Config.DeleteAfter.Duration <= 0:
			// if DeleteAfter is 0 we don't delete anything. we are done.
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, false)
			u.unlockHistory()
			delete(u.folders.Folders, name)
		case EXTRACTED == folder.Status && elapsed >= folder.Config.DeleteAfter.Duration:
			u.deleteAfterReached(name, now, folder)
		}
	}
}

//nolint:wsl_v5
func (u *Unpackerr) deleteAfterReached(name string, now time.Time, folder *Folder) {
	var webhook bool
	// Folder reached delete delay (after extraction), nuke it.
	if folder.Config.DeleteFiles && !folder.Config.MoveBack {
		u.queueDelete(&fileDeleteReq{Paths: []string{strings.TrimRight(name, `/\`) + suffix}})
		webhook = true
	} else if folder.Config.DeleteFiles && len(folder.Files) > 0 {
		u.queueDelete(&fileDeleteReq{Paths: folder.Files})
		webhook = true
	}

	if folder.Config.DeleteOrig && !folder.Config.MoveBack {
		u.queueDelete(&fileDeleteReq{Paths: []string{name}})
		webhook = true
	} else if folder.Config.DeleteOrig && len(folder.Archives) > 0 {
		u.queueDelete(&fileDeleteReq{Paths: folder.Archives.List()})
		webhook = true
	}

	u.lockHistory()
	u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, webhook)
	u.unlockHistory()
	// Folder reached delete delay (after extraction), nuke it.
	delete(u.folders.Folders, name)
}

type newStatus struct {
	Name   string
	Status ExtractStatus
	Resp   *xtractr.Response
}

// updateQueueStatus for an on-going tracked extraction.
// This is called from a channel callback to update status in a single go routine.
// This is used by apps and Folders in a few other places as well.
func (u *Unpackerr) updateQueueStatus(data *newStatus, now time.Time, sendHook bool) *Extract {
	if _, ok := u.Map[data.Name]; !ok {
		// This is a new Folder item. Arr apps do not land here.
		// They create their own queued items in u.Map.
		u.Map[data.Name] = &Extract{
			Path:    data.Name,
			App:     FolderString,
			Status:  data.Status,
			Updated: now,
			IDs:     map[string]any{"title": data.Name}, // required or webhook may break.
		}

		u.Map[data.Name].XProg = &ExtractProgress{Extract: u.Map[data.Name]}

		if sendHook {
			u.runAllHooks(u.Map[data.Name])
		}

		return u.Map[data.Name]
	}

	if data.Resp != nil {
		u.Map[data.Name].Resp = data.Resp
	}

	u.Map[data.Name].Status = data.Status
	u.Map[data.Name].Updated = now

	if sendHook {
		u.runAllHooks(u.Map[data.Name])
	}

	u.maybeRecordHistory(data.Name, u.Map[data.Name])

	return u.Map[data.Name]
}
