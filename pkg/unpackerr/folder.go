package unpackerr

/* Folder Watching Codez */

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/fsnotify/fsnotify"
	"github.com/radovskyb/watcher"
	"golift.io/cnfg"
	"golift.io/xtractr"
)

// defaultPollInterval is used if Docker is detected.
const (
	defaultPollInterval = time.Second
	minimumPollInterval = 5 * time.Millisecond
	defaultFolderDelete = 10 * time.Minute
)

// FolderConfig defines the input data for a watched folder.
//
//nolint:lll
type FolderConfig struct {
	DeleteOrig       bool           `json:"delete_original"  toml:"delete_original"   xml:"delete_original"   yaml:"delete_original"`
	DeleteFiles      bool           `json:"delete_files"     toml:"delete_files"      xml:"delete_files"      yaml:"delete_files"`
	DisableLog       bool           `json:"disable_log"      toml:"disable_log"       xml:"disable_log"       yaml:"disable_log"`
	MoveBack         bool           `json:"move_back"        toml:"move_back"         xml:"move_back"         yaml:"move_back"`
	DeleteAfter      *cnfg.Duration `json:"delete_after"     toml:"delete_after"      xml:"delete_after"      yaml:"delete_after"`
	ExtractPath      string         `json:"extract_path"     toml:"extract_path"      xml:"extract_path"      yaml:"extract_path"`
	ExtractISOs      bool           `json:"extract_isos"     toml:"extract_isos"      xml:"extract_isos"      yaml:"extract_isos"`
	DisableRecursion bool           `json:"disableRecursion" toml:"disable_recursion" xml:"disable_recursion" yaml:"disableRecursion"`
	MaxNested        int            `json:"maxNested"        toml:"max_nested"        xml:"max_nested"        yaml:"maxNested"`
	ExtrasMaxDepth   int            `json:"extrasMaxDepth"   toml:"extras_max_depth"  xml:"extras_max_depth"  yaml:"extrasMaxDepth"`
	AllowSymlinks    bool           `json:"allowSymlinks"    toml:"allow_symlinks"    xml:"allow_symlinks"    yaml:"allowSymlinks"`
	MaxBytes         string         `json:"maxBytes"         toml:"max_bytes"         xml:"max_bytes"         yaml:"maxBytes"`
	MaxFiles         int            `json:"maxFiles"         toml:"max_files"         xml:"max_files"         yaml:"maxFiles"`
	MaxRatio         float64        `json:"maxRatio"         toml:"max_ratio"         xml:"max_ratio"         yaml:"maxRatio"`
	// maxBytes is 0 when unset: folder watcher is uncapped.
	maxBytes     uint64
	ExcludePaths []string `json:"exclude_paths" toml:"exclude_paths" xml:"exclude_path" yaml:"exclude_paths"`
	Path         string   `json:"path"          toml:"path"          xml:"path"         yaml:"path"`
}

// Folders holds all known (created) folders in all watch paths.
type Folders struct {
	Logs
	Interval time.Duration
	Config   []*FolderConfig
	Folders  map[string]*Folder
	Events   chan *eventData
	Updates  chan *xtractr.Response
	FSNotify *fsnotify.Watcher
	Watcher  *watcher.Watcher
}

// Logs interface for folders.
type Logs interface {
	Printf(msg string, v ...any)
	Errorf(msg string, v ...any)
	Debugf(msg string, v ...any)
}

// Folder is a "new" watched folder.
type Folder struct {
	updated  time.Time
	status   ExtractStatus
	config   *FolderConfig
	files    []string
	retries  uint
	archives xtractr.ArchiveList
	// preFiles is the snapshot of each archive dest before extraction
	// (MoveBack only). Dest folders come from FindCompressedFiles so nested
	// archive dirs are included. Kept across retries so failed cleanups are
	// not recaptured as download content. Nil means remnant handling is skipped.
	preFiles map[string]os.FileInfo
	// noRetry is set when remnant_action=off leaves a blocker.
	noRetry bool
}

type eventData struct {
	cnfg *FolderConfig
	name string
	file string
	op   string
}

func (u *Unpackerr) validateFolders() error {
	for idx := range u.Folders {
		if u.Folders[idx].DeleteAfter == nil {
			// If delete after wasn't set, then set it to 10 minutes.
			u.Folders[idx].DeleteAfter = &cnfg.Duration{Duration: defaultFolderDelete}
		}

		n, _, err := parseOptionalMaxBytes(u.Folders[idx].MaxBytes)
		if err != nil {
			return fmt.Errorf("folder %s: %w", u.Folders[idx].Path, err)
		}

		u.Folders[idx].maxBytes = n
	}

	return nil
}

func (u *Unpackerr) logFolders() {
	if epath, count := "", len(u.Folders); count == 1 {
		folder := u.Folders[0]
		if folder.ExtractPath != "" {
			epath = ", extract to: " + folder.ExtractPath
		}

		u.Printf(" => Folder Config: 1 path: %s%s; delete_after:%v delete_orig:%v delete_files:%v "+
			"log_file:%v move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v event_buffer:%d",
			folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
			!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
			folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks, u.Folder.Buffer)
	} else {
		u.Printf(" => Folder Config: %d paths, event_buffer:%d ", count, u.Folder.Buffer)

		for _, folder := range u.Folders {
			if epath = ""; folder.ExtractPath != "" {
				epath = " extract to: " + folder.ExtractPath
			}

			u.Printf(" =>    Path: %s%s; delete_after:%v delete_orig:%v delete_files:%v log_file:%v "+
				"move_back:%v isos:%v files:%d ratio:%g nested:%d extras_depth:%d symlinks:%v",
				folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, folder.DeleteFiles,
				!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, folder.MaxFiles, folder.MaxRatio,
				folder.MaxNested, folder.ExtrasMaxDepth, folder.AllowSymlinks)
		}
	}
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
		u.Folder.Interval.Duration = defaultPollInterval
	}

	u.Folders, flist = checkFolders(u.Folders, u.Logger)

	u.folders, err = u.Folder.newWatcher(u.Folders, u.Logger)
	if err != nil {
		u.Errorf("Watching Folders: %s", err)
		return
	}
	// do not close either watcher.

	if len(u.Folders) == 0 {
		return
	}

	go u.folders.watchFSNotify()

	u.Printf("[Folder] Watching (fsnotify): %s", strings.Join(flist, ", "))

	// Setting an interval of any value less than 5 milliseconds
	// (except zero in docker) allows disabling the poller.
	if u.Folder.Interval.Duration < minimumPollInterval {
		return
	}

	go func() {
		if err := u.folders.Watcher.Start(u.Folder.Interval.Duration); err != nil {
			u.Errorf("Folder poller stopped: %v", err)
		}
	}()

	u.Printf("[Folder] Polling @ %s: %s", u.Folder.Interval.String(), strings.Join(flist, ", "))
}

// checkFolders stats all configured folders and returns only "good" ones.
func checkFolders(folders []*FolderConfig, log Logs) ([]*FolderConfig, []string) {
	var (
		err         error
		goodFolders = folders[:0]
		goodFlist   = []string{}
	)

	for _, folder := range folders {
		folder.Path, err = filepath.Abs(expandHomedir(folder.Path))
		if err != nil {
			log.Errorf("Folder '%s' (bad path): %v", folder.Path, err)
			continue
		}

		if folder.ExtractPath != "" {
			folder.ExtractPath, err = filepath.Abs(expandHomedir(folder.ExtractPath))
			if err != nil {
				log.Errorf("Folder '%s' (bad extract path): %v", folder.ExtractPath, err)
				continue
			}
		}

		folder.ExcludePaths = normalizeFolderExcludePaths(folder.Path, folder.ExcludePaths)

		if stat, err := os.Stat(folder.Path); err != nil {
			log.Errorf("Folder '%s' (cannot watch): %v", folder.Path, err)
			continue
		} else if !stat.IsDir() {
			log.Errorf("Folder '%s' (cannot watch): not a folder", folder.Path)
			continue
		}

		goodFolders = append(goodFolders, folder)
		goodFlist = append(goodFlist, folder.Path)
	}

	return goodFolders, goodFlist
}

func normalizeFolderExcludePaths(basePath string, excludes []string) []string {
	cleaned := make([]string, 0, len(excludes))

	for _, exclude := range excludes {
		exclude = strings.TrimSpace(exclude)
		if exclude == "" {
			continue
		}

		exclude = expandHomedir(exclude)
		if !filepath.IsAbs(exclude) {
			exclude = filepath.Join(basePath, exclude)
		}

		if abs, err := filepath.Abs(exclude); err == nil {
			cleaned = append(cleaned, filepath.Clean(abs))
		}
	}

	return cleaned
}

func (c *FolderConfig) isExcludedPath(path string) bool {
	if len(c.ExcludePaths) == 0 || path == "" {
		return false
	}

	path = filepath.Clean(path)

	for _, exclude := range c.ExcludePaths {
		exclude = filepath.Clean(exclude)
		if path == exclude || strings.HasPrefix(path, exclude+string(os.PathSeparator)) {
			return true
		}
	}

	return false
}

// newWatcher returns a new folder watcher.
// You must call folders.FSNotify.Close() when you're done with it.
func (c FoldersConfig) newWatcher(folderConfig []*FolderConfig, log Logs) (*Folders, error) {
	folders := &Folders{
		Interval: c.Interval.Duration,
		Config:   folderConfig,
		Folders:  make(map[string]*Folder),
		Events:   make(chan *eventData, c.Buffer),
		Updates:  make(chan *xtractr.Response, updateChanBuf),
		Logs:     log,
	}

	if len(folderConfig) == 0 {
		return folders, nil // do not initialize watcher
	}

	folders.Watcher = watcher.New()
	folders.Watcher.FilterOps(watcher.Rename, watcher.Move, watcher.Write, watcher.Create)
	folders.Watcher.IgnoreHiddenFiles(true)

	fsn, err := fsnotify.NewWatcher()
	if err != nil {
		return folders, fmt.Errorf("fsnotify.NewWatcher: %w", err)
	}

	folders.FSNotify = fsn

	for _, folder := range folderConfig {
		if err := folders.Watcher.Add(folder.Path); err != nil {
			log.Errorf("Folder '%s' (cannot poll): %v", folder.Path, err)
		}

		if err := fsn.Add(folder.Path); err != nil {
			log.Errorf("Folder '%s' (cannot watch): %v", folder.Path, err)
		}
	}

	return folders, nil
}

// Add uses either fsnotify or watcher.
func (f *Folders) Add(folder string) error {
	if f.Interval >= minimumPollInterval {
		if err := f.Watcher.Add(folder); err != nil {
			return fmt.Errorf("watcher: %w", err)
		}

		return nil
	}

	if err := f.FSNotify.Add(folder); err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}

	return nil
}

// Remove uses either fsnotify or watcher.
func (f *Folders) Remove(folder string) {
	if f.Watcher != nil {
		_ = f.Watcher.Remove(folder)
	}

	if f.FSNotify != nil {
		_ = f.FSNotify.Remove(folder)
	}
}

// extractTrackedItem starts an archive or folder's extraction after it hasn't been written to in a while.
func (u *Unpackerr) extractTrackedItem(name string, folder *Folder, now time.Time) {
	u.folders.Remove(name) // stop the fs watcher(s).
	// update status.
	u.folders.Folders[name].updated = now
	u.folders.Folders[name].status = QUEUED

	// Do not extract r00 file if rar file with same name exists.
	if strings.HasSuffix(strings.ToLower(name), ".r00") &&
		xtractr.CheckR00ForRarFile(getFileList(filepath.Dir(name)), filepath.Base(name)) {
		u.Printf("[Folder] Removing tracked item without extraction: %v (rar file exists)", name)
		u.folders.Folders[name].status = EXTRACTEDNOTHING

		return
	}

	// create a queue counter in the main history; add to u.Map and send webhook for a new folder.
	u.lockHistory()
	item := u.updateQueueStatus(&newStatus{Name: name, Status: QUEUED}, u.folders.Folders[name].updated, true)
	u.unlockHistory()
	u.updateHistory(FolderString + ": " + name)

	exclude := folderExcludeSuffixes(name, folder.config)

	if folder.config.MoveBack {
		found := xtractr.FindCompressedFiles(xtractr.Filter{
			Path:          name,
			ExcludeSuffix: exclude,
			AllowSymlinks: folder.config.AllowSymlinks,
		})

		snap, err := keepDirSnapshot(folder.preFiles, archiveSnapshotPaths(name, found)...)
		if err != nil {
			u.Errorf("[Folder] Snapshot dests for remnant check: %v", err)
		} else {
			folder.preFiles = snap
		}
	}

	// extract it.
	queueSize, err := u.Extract(&xtractr.Xtract{
		Password:         u.getPasswordFromPath(name),
		Passwords:        u.Passwords,
		Name:             name,
		Path:             name,
		ExcludeSuffix:    exclude,
		AllowSymlinks:    folder.config.AllowSymlinks,
		MaxBytes:         folder.config.maxBytes,
		MaxFiles:         folder.config.MaxFiles,
		MaxRatio:         folder.config.MaxRatio,
		MaxNested:        folder.config.MaxNested,
		ExtrasMaxDepth:   folder.config.ExtrasMaxDepth,
		TempFolder:       !folder.config.MoveBack,
		ExtractTo:        folder.config.ExtractPath,
		DeleteOrig:       false,
		CBChannel:        u.folders.Updates,
		CBFunction:       nil,
		Progress:         u.progressUpdateCallback(item),
		LogFile:          !folder.config.DisableLog,
		DisableRecursion: folder.config.DisableRecursion,
	})
	if err != nil {
		u.Errorf("[ERROR] %v", err)
		return
	}

	u.Printf("[Folder] Queued: %s, queue size: %d", name, queueSize)
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
		folder.status = EXTRACTING
		u.Printf("[Folder] Extraction Started: %s, retries: %d, items in queue: %d", resp.X.Name, folder.retries, resp.Queued)
		folder.updated = now
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.status}, folder.updated, true)
		u.unlockHistory()

		return
	}

	if errors.Is(resp.Error, xtractr.ErrNoCompressedFiles) {
		folder.status = EXTRACTEDNOTHING
		u.Printf("[Folder] %s: %s: %v", folder.status.Desc(), resp.X.Name, resp.Error)
		folder.updated = now
		u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.status}, folder.updated, true)
		u.unlockHistory()

		return
	}

	preFiles := folder.preFiles
	retries := folder.retries
	configPath := folder.config.Path

	u.unlockHistory()

	remnantStatus, remnants := u.finishFolderExtractWork(resp, preFiles, retries, configPath)

	u.lockHistory()
	defer u.unlockHistory()

	folder, found = u.folders.Folders[resp.X.Name]
	if !found {
		return
	}

	u.commitFolderExtract(folder, resp, remnantStatus, remnants)
	folder.updated = now
	u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.status}, folder.updated, true)
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
		folder.archives = resp.Archives
	}

	if remnants {
		u.finishFolderRemnants(folder, resp, status)
		return
	}

	if resp.Error != nil {
		folder.status = EXTRACTFAILED
		return
	}

	folder.archives = resp.Archives
	folder.status = EXTRACTED
	folder.files = resp.NewFiles
}

func (u *Unpackerr) finishFolderRemnants(folder *Folder, resp *xtractr.Response, status ExtractStatus) {
	if status == WAITING {
		u.Printf("[Folder] Cleared interrupted-extraction remnant(s), restarting extraction: %s", resp.X.Name)

		folder.status = EXTRACTFAILED

		return
	}

	if remnantAction(u.RemnantAction) == "off" {
		folder.noRetry = true
	}

	u.Errorf("[Folder] Extraction blocked by interrupted-extraction remnant(s): %s", resp.X.Name)

	folder.status = EXTRACTFAILED
}

// watchFSNotify reads file system events from a channel and processes them.
// This runs in its own go routine, and eventually sends the event back into the main routine.
func (f *Folders) watchFSNotify() {
	defer log.Println("Folder watcher routine exited. No longer watching any folders.")

	for {
		select {
		case err := <-f.Watcher.Error:
			f.Errorf("watcher: %v", err)
		case err := <-f.FSNotify.Errors:
			f.Errorf("fsnotify: %v", err)
		case event, ok := <-f.FSNotify.Events:
			if !ok {
				return
			}

			f.handleFileEvent(event.Name, "f "+event.Op.String())
		case event := <-f.Watcher.Event:
			f.handleFileEvent(event.Path, "w "+event.Op.String())
		case <-f.Watcher.Closed:
			return
		}
	}
}

// handleFileEvent takes formatted events from fsnotify or fswatcher, does minimal
// (thread safe) validation before sending the re-formatted event to the main go routine.
func (f *Folders) handleFileEvent(name, operation string) {
	if strings.HasSuffix(name, suffix) {
		return
	}

	// Send this event to processEvent().
	for _, cnfg := range f.Config {
		// Do not handle events on the watched folder itself.
		if name == cnfg.Path {
			return
		}

		// cnfg.Path: "/Users/Documents/watched_folder"
		// event.Name: "/Users/Documents/watched_folder/new_folder/file.rar"
		// eventData.name: "new_folder"
		if !strings.HasPrefix(name, cnfg.Path) {
			continue // Not the configured folder for the event we just got.
		}

		if cnfg.isExcludedPath(name) {
			f.Debugf("Folder: Ignored event from excluded path: %v", name)
			continue
		}

		// processEvent (below) handles events sent to f.Events.
		if dir := filepath.Dir(name); dir == cnfg.Path {
			f.Events <- &eventData{name: filepath.Base(name), cnfg: cnfg, file: name, op: operation}
		} else {
			f.Events <- &eventData{name: filepath.Base(dir), cnfg: cnfg, file: name, op: operation}
		}

		return
	}

	f.Debugf("Folder: Ignored event from non-configured path: %v", name)
}

// processEvent is here to process the event in the `*Unpackerr` scope before sending it back to the `*Folders` scope.
func (u *Unpackerr) processEvent(event *eventData, now time.Time) {
	// Do not watch our own log file.
	if event.file == u.LogFile || event.file == u.Webserver.LogFile {
		return
	}

	u.folders.processEvent(event, now)
}

// processEvent processes the event that was received.
func (f *Folders) processEvent(event *eventData, now time.Time) {
	dirPath := filepath.Join(event.cnfg.Path, event.name)

	if event.cnfg.isExcludedPath(event.file) || event.cnfg.isExcludedPath(dirPath) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (excluded path)", event.op, event.file)
		return
	}

	stat, err := os.Stat(dirPath)
	if err != nil {
		// Item is unusable (probably deleted), remove it from history.
		if _, ok := f.Folders[dirPath]; ok {
			f.Debugf("Folder: Removing Tracked Item: %v", dirPath)
			delete(f.Folders, dirPath)
			f.Remove(dirPath)
		}

		f.Debugf("Folder: Ignored File Event (%s) '%s' (unreadable): %v", event.op, event.file, err)

		return
	}

	if !stat.IsDir() && !xtractr.IsArchiveFile(event.name) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (not archive or dir): %v", event.op, event.file, err)
		return
	}

	f.saveEvent(event, dirPath, now)
}

func (f *Folders) saveEvent(event *eventData, dirPath string, now time.Time) {
	if _, ok := f.Folders[dirPath]; ok {
		// f.Debugf("Item Updated: %v", event.file)
		f.Folders[dirPath].updated = now
		return
	}

	if err := f.Add(dirPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			f.Errorf("Folder: Tracking New Item: %v (event: %s): %v ", dirPath, event.op, err)
		}

		return
	}

	f.Printf("[Folder] Tracking New Item: %v (event: %s)", dirPath, event.op)

	f.Folders[dirPath] = &Folder{
		updated: now,
		status:  WAITING,
		config:  event.cnfg,
	}
}

// checkFolderStats runs at an interval to see if any folders need work done on them.
// This runs on an interval ticker in the main go routine.
func (u *Unpackerr) checkFolderStats(now time.Time) {
	for name, folder := range u.folders.Folders {
		switch elapsed := now.Sub(folder.updated); {
		case WAITING == folder.status && elapsed >= u.StartDelay.Duration:
			// The folder hasn't been written to in a while, extract it.
			u.extractTrackedItem(name, folder, now)
		case EXTRACTEDNOTHING == folder.status:
			// Wait until this item hasn't been touched for a while, so it doesn't re-queue.
			if now.Sub(folder.updated) > u.StartDelay.Duration {
				// Ignore "no compressed files" errors for folders.
				u.lockHistory()
				delete(u.Map, name)
				u.unlockHistory()
				delete(u.folders.Folders, name)
			}
		case EXTRACTFAILED == folder.status && folder.noRetry:
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, true)
			u.unlockHistory()
			delete(u.folders.Folders, name)
			u.Printf("[Folder] Remnant left in place (remnant_action=off), giving up: %s", name)
		case EXTRACTFAILED == folder.status && elapsed >= u.RetryDelay.Duration &&
			folder.retries < u.maxRetries():
			u.lockHistory()
			u.Retries++
			u.unlockHistory()

			folder.retries++
			folder.updated = now
			folder.status = WAITING
			u.Printf("[Folder] Re-starting Failed Extraction: %s (%d/%d, failed %v ago)",
				folder.config.Path, folder.retries, u.maxRetries(), elapsed.Round(time.Second))
		case EXTRACTFAILED == folder.status && folder.retries < u.maxRetries():
			// This empty block is to avoid deleting an item that needs more retries.
		case EXTRACTFAILED == folder.status && folder.retries >= u.maxRetries():
			// Retries exhausted — clean up to prevent the item from staying in the map forever.
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, true)
			u.unlockHistory()
			delete(u.folders.Folders, name)
			u.Printf("[Folder] Retries exhausted (%d/%d), giving up: %s", folder.retries, u.maxRetries(), name)
		case EXTRACTED == folder.status && folder.config.DeleteAfter.Duration <= 0:
			// if DeleteAfter is 0 we don't delete anything. we are done.
			u.lockHistory()
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, now, false)
			u.unlockHistory()
			delete(u.folders.Folders, name)
		case EXTRACTED == folder.status && elapsed >= folder.config.DeleteAfter.Duration:
			u.deleteAfterReached(name, now, folder)
		}
	}
}

//nolint:wsl_v5
func (u *Unpackerr) deleteAfterReached(name string, now time.Time, folder *Folder) {
	var webhook bool
	// Folder reached delete delay (after extraction), nuke it.
	if folder.config.DeleteFiles && !folder.config.MoveBack {
		u.delChan <- &fileDeleteReq{Paths: []string{strings.TrimRight(name, `/\`) + suffix}}
		webhook = true
	} else if folder.config.DeleteFiles && len(folder.files) > 0 {
		u.delChan <- &fileDeleteReq{Paths: folder.files}
		webhook = true
	}

	if folder.config.DeleteOrig && !folder.config.MoveBack {
		u.delChan <- &fileDeleteReq{Paths: []string{name}}
		webhook = true
	} else if folder.config.DeleteOrig && len(folder.archives) > 0 {
		u.delChan <- &fileDeleteReq{Paths: folder.archives.List()}
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
		// This is a new Folder being queued for extraction.
		// Arr apps do not land here. They create their own queued items in u.Map.
		u.Map[data.Name] = &Extract{
			Path:    data.Name,
			App:     FolderString,
			Status:  QUEUED,
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

	u.maybeRecordHistory(u.Map[data.Name])

	return u.Map[data.Name]
}
