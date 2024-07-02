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

	"github.com/fsnotify/fsnotify"
	"github.com/radovskyb/watcher"
	"golift.io/cnfg"
	"golift.io/xtractr"
)

// defaultPollInterval is used if Docker is detected.
const (
	defaultPollInterval = time.Second
	minimumPollInterval = 5 * time.Millisecond
)

// FolderConfig defines the input data for a watched folder.
//
//nolint:lll
type FolderConfig struct {
	DeleteOrig       bool          `json:"delete_original" toml:"delete_original" xml:"delete_original" yaml:"delete_original"`
	DeleteFiles      bool          `json:"delete_files" toml:"delete_files" xml:"delete_files" yaml:"delete_files"`
	DisableLog       bool          `json:"disable_log" toml:"disable_log" xml:"disable_log" yaml:"disable_log"`
	MoveBack         bool          `json:"move_back" toml:"move_back" xml:"move_back" yaml:"move_back"`
	DeleteAfter      cnfg.Duration `json:"delete_after" toml:"delete_after" xml:"delete_after" yaml:"delete_after"`
	ExtractPath      string        `json:"extract_path" toml:"extract_path" xml:"extract_path" yaml:"extract_path"`
	ExtractISOs      bool          `json:"extract_isos" toml:"extract_isos" xml:"extract_isos" yaml:"extract_isos"`
	DisableRecursion bool          `json:"disableRecursion" toml:"disable_recursion" xml:"disable_recursion" yaml:"disableRecursion"`
	Path             string        `json:"path" toml:"path" xml:"path" yaml:"path"`
}

// Folders holds all known (created) folders in all watch paths.
type Folders struct {
	Interval time.Duration
	Config   []*FolderConfig
	Folders  map[string]*Folder
	Events   chan *eventData
	Updates  chan *xtractr.Response
	FSNotify *fsnotify.Watcher
	Watcher  *watcher.Watcher
	Logs
}

// Logs interface for folders.
type Logs interface {
	Printf(msg string, v ...interface{})
	Errorf(msg string, v ...interface{})
	Debugf(msg string, v ...interface{})
}

// Folder is a "new" watched folder.
type Folder struct {
	updated  time.Time
	status   ExtractStatus
	config   *FolderConfig
	files    []string
	retries  uint
	archives []string
}

type eventData struct {
	cnfg *FolderConfig
	name string
	file string
	op   string
}

func (u *Unpackerr) logFolders() {
	if epath, count := "", len(u.Folders); count == 1 {
		folder := u.Folders[0]
		if folder.ExtractPath != "" {
			epath = ", extract to: " + folder.ExtractPath
		}

		u.Printf(" => Folder Config: 1 path: %s%s (delete after:%v, delete orig:%v, "+
			"log file: %v, move back:%v, isos:%v, event buffer:%d)",
			folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig,
			!folder.DisableLog, folder.MoveBack, folder.ExtractISOs, u.Folder.Buffer)
	} else {
		u.Printf(" => Folder Config: %d paths, event buffer: %d ", count, u.Folder.Buffer)

		for _, folder := range u.Folders {
			if epath = ""; folder.ExtractPath != "" {
				epath = ", extract to: " + folder.ExtractPath
			}

			u.Printf(" =>    Path: %s%s (delete after:%v, delete orig:%v, log file: %v, move back:%v, isos:%v)",
				folder.Path, epath, folder.DeleteAfter, folder.DeleteOrig, !folder.DisableLog, folder.MoveBack, folder.ExtractISOs)
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

	// Setting an interval of any value less than a millisecond
	// (except zero in docker) allows disabling the poller.
	if u.Folder.Interval.Duration < minimumPollInterval {
		return
	}

	go func() {
		if err := u.folders.Watcher.Start(u.Folder.Interval.Duration); err != nil {
			u.Errorf("Folder poller stopped: %v", err)
		}
	}()
	u.Printf("[Folder] Polling @ %v: %s", u.Folder.Interval, strings.Join(flist, ", "))
}

// checkFolders stats all configured folders and returns only "good" ones.
func checkFolders(folders []*FolderConfig, log Logs) ([]*FolderConfig, []string) {
	goodFolders := []*FolderConfig{}
	goodFlist := []string{}

	for _, folder := range folders {
		path, err := filepath.Abs(folder.Path)
		if err != nil {
			log.Errorf("Folder '%s' (bad path): %v", folder.Path, err)
			continue
		}

		folder.Path = path // rewrite it. might not be safe.
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
func (u *Unpackerr) extractTrackedItem(name string, folder *Folder) {
	u.folders.Remove(name) // stop the fs watcher(s).
	// update status.
	u.folders.Folders[name].updated = time.Now()
	u.folders.Folders[name].status = QUEUED

	// Do not extract r00 file if rar file with same name exists.
	if strings.HasSuffix(strings.ToLower(name), ".r00") &&
		xtractr.CheckR00ForRarFile(getFileList(filepath.Dir(name)), filepath.Base(name)) {
		u.Printf("[Folder] Removing tracked item without extraction: %v (rar file exists)", name)
		u.folders.Folders[name].status = EXTRACTEDNOTHING

		return
	}

	// create a queue counter in the main history; add to u.Map and send webhook for a new folder.
	u.updateQueueStatus(&newStatus{Name: name, Status: QUEUED}, true)
	u.updateHistory(FolderString + ": " + name)

	var exclude []string
	if !folder.config.ExtractISOs {
		exclude = append(exclude, ".iso")
	}

	// extract it.
	queueSize, err := u.Extract(&xtractr.Xtract{
		Password:         u.getPasswordFromPath(name),
		Passwords:        u.Passwords,
		Name:             name,
		Filter:           xtractr.Filter{Path: name, ExcludeSuffix: exclude},
		TempFolder:       !folder.config.MoveBack,
		ExtractTo:        folder.config.ExtractPath,
		DeleteOrig:       false,
		CBChannel:        u.folders.Updates,
		CBFunction:       nil,
		LogFile:          !folder.config.DisableLog,
		DisableRecursion: folder.config.DisableRecursion,
	})
	if err != nil {
		u.Errorf("[ERROR] %v", err)
		return
	}

	u.Printf("[Folder] Queued: %s, queue size: %d", name, queueSize)
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
	folder, ok := u.folders.Folders[resp.X.Name]

	switch {
	case !ok:
		// It doesn't exist? weird. delete it and bail out.
		delete(u.Map, resp.X.Name)
		return
	case !resp.Done:
		folder.status = EXTRACTING
		u.Printf("[Folder] Extraction Started: %s, retries: %d, items in queue: %d", resp.X.Name, folder.retries, resp.Queued)
	case errors.Is(resp.Error, xtractr.ErrNoCompressedFiles):
		folder.status = EXTRACTEDNOTHING
		u.Printf("[Folder] %s: %s: %v", folder.status.Desc(), resp.X.Name, resp.Error)
	case resp.Error != nil:
		folder.status = EXTRACTFAILED
		u.Errorf("[Folder] %s: %s: %v", folder.status.Desc(), resp.X.Name, resp.Error)
		u.updateMetrics(resp, FolderString, folder.config.Path)

		for _, v := range resp.Archives {
			folder.archives = append(folder.archives, v...)
		}
	default: // this runs in a go routine
		for _, v := range resp.Archives {
			folder.archives = append(folder.archives, v...)
		}

		u.updateMetrics(resp, FolderString, folder.config.Path)
		u.Printf("[Folder] Extraction Finished: %s => elapsed: %v, archives: %d, "+
			"extra archives: %d, files extracted: %d, written: %dMiB",
			resp.X.Name, resp.Elapsed.Round(time.Second), len(folder.archives),
			mapLen(resp.Extras), len(resp.NewFiles), resp.Size/mebiByte)

		folder.status = EXTRACTED
		folder.files = resp.NewFiles
	}

	folder.updated = time.Now()

	u.updateQueueStatus(&newStatus{Name: resp.X.Name, Resp: resp, Status: folder.status}, true)
}

func mapLen(in map[string][]string) (out int) {
	for _, v := range in {
		out += len(v)
	}

	return out
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

		// processEvent (below) handles events sent to f.Events.
		if p := filepath.Dir(name); p == cnfg.Path {
			f.Events <- &eventData{name: filepath.Base(name), cnfg: cnfg, file: name, op: operation}
		} else {
			f.Events <- &eventData{name: filepath.Base(p), cnfg: cnfg, file: name, op: operation}
		}

		return
	}

	f.Debugf("Folder: Ignored event from non-configured path: %v", name)
}

// processEvent is here to process the event in the `*Unpackerr` scope before sending it back to the `*Folders` scope.
func (u *Unpackerr) processEvent(event *eventData) {
	// Do not watch our own log file.
	if event.file == u.Config.LogFile || event.file == u.Config.Webserver.LogFile {
		return
	}

	u.folders.processEvent(event)
}

// processEvent processes the event that was received.
func (f *Folders) processEvent(event *eventData) {
	dirPath := filepath.Join(event.cnfg.Path, event.name)

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

	f.saveEvent(event, dirPath)
}

func (f *Folders) saveEvent(event *eventData, dirPath string) {
	if _, ok := f.Folders[dirPath]; ok {
		// f.Debugf("Item Updated: %v", event.file)
		f.Folders[dirPath].updated = time.Now()
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
		updated: time.Now(),
		status:  WAITING,
		config:  event.cnfg,
	}
}

// checkFolderStats runs at an interval to see if any folders need work done on them.
// This runs on an interval ticker in the main go routine.
func (u *Unpackerr) checkFolderStats() {
	for name, folder := range u.folders.Folders {
		switch elapsed := time.Since(folder.updated); {
		case WAITING == folder.status && elapsed >= u.StartDelay.Duration:
			// The folder hasn't been written to in a while, extract it.
			u.extractTrackedItem(name, folder)
		case EXTRACTEDNOTHING == folder.status:
			// Wait until this item hasn't been touched for a while, so it doesn't re-queue.
			if time.Since(folder.updated) > u.StartDelay.Duration {
				// Ignore "no compressed files" errors for folders.
				delete(u.Map, name)
				delete(u.folders.Folders, name)
			}
		case EXTRACTFAILED == folder.status && elapsed >= u.RetryDelay.Duration &&
			(u.MaxRetries == 0 || folder.retries < u.MaxRetries):
			u.Retries++
			folder.retries++
			folder.updated = time.Now()
			folder.status = WAITING
			u.Printf("[Folder] Re-starting Failed Extraction: %s (%d/%d, failed %v ago)",
				folder.config.Path, folder.retries, u.MaxRetries, elapsed.Round(time.Second))
		case EXTRACTFAILED == folder.status && folder.retries < u.MaxRetries:
			// This empty block is to avoid deleting an item that needs more retries.
		case folder.status > EXTRACTING && folder.config.DeleteAfter.Duration <= 0:
			// if DeleteAfter is 0 we don't delete anything. we are done.
			u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, false)
			delete(u.folders.Folders, name)
		case EXTRACTED == folder.status && elapsed >= folder.config.DeleteAfter.Duration:
			u.deleteAfterReached(name, folder)
		}
	}
}

//nolint:wsl
func (u *Unpackerr) deleteAfterReached(name string, folder *Folder) {
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
		u.delChan <- &fileDeleteReq{Paths: folder.archives}
		webhook = true
	}

	u.updateQueueStatus(&newStatus{Name: name, Status: DELETED, Resp: nil}, webhook)
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
func (u *Unpackerr) updateQueueStatus(data *newStatus, sendHook bool) {
	if _, ok := u.Map[data.Name]; !ok {
		// This is a new Folder being queued for extraction.
		// Arr apps do not land here. They create their own queued items in u.Map.
		u.Map[data.Name] = &Extract{
			Path:    data.Name,
			App:     FolderString,
			Status:  QUEUED,
			Updated: time.Now(),
			IDs:     map[string]interface{}{"title": data.Name}, // required or webhook may break.
		}

		if sendHook {
			u.runAllHooks(u.Map[data.Name])
		}

		return
	}

	if data.Resp != nil {
		u.Map[data.Name].Resp = data.Resp
	}

	u.Map[data.Name].Status = data.Status
	u.Map[data.Name].Updated = time.Now()

	if sendHook {
		u.runAllHooks(u.Map[data.Name])
	}
}
