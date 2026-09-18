package folders

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/fsnotify/fsnotify"
	"github.com/radovskyb/watcher"
	"golift.io/xtractr"
)

// NewWatcher returns a new folder watcher.
// Call Close() when you are done with it (tests). The daemon leaves it open.
func (c WatchConfig) NewWatcher(
	folderConfig []*FolderConfig,
	logger Logs,
	updateBuf int,
	ignoreSuffix string,
) (*Folders, error) {
	folders := &Folders{
		Config:       folderConfig,
		Folders:      make(map[string]*Folder),
		Events:       make(chan *Event, c.Buffer),
		Updates:      make(chan *xtractr.Response, updateBuf),
		Logs:         logger,
		IgnoreSuffix: ignoreSuffix,
	}

	if len(folderConfig) == 0 {
		return folders, nil // do not initialize watcher
	}

	if folders.addPollers(folderConfig, logger) {
		if err := folders.openFSNotify(folderConfig, logger); err != nil {
			folders.Close()

			return folders, err
		}
	}

	return folders, nil
}

func (f *Folders) addPollers(folderConfig []*FolderConfig, logger Logs) bool {
	needFSNotify := false

	for _, folder := range folderConfig {
		if folder == nil {
			continue
		}

		if !folder.UsesPoller() {
			needFSNotify = true
			continue
		}

		poller, err := newFolderPoller(folder)
		if err != nil {
			logger.Errorf("Folder '%s' (cannot poll, using fsnotify): %v", folder.Path, err)

			needFSNotify = true

			continue
		}

		f.pollers = append(f.pollers, poller)
	}

	return needFSNotify
}

func (f *Folders) openFSNotify(folderConfig []*FolderConfig, logger Logs) error {
	fsn, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify.NewWatcher: %w", err)
	}

	f.FSNotify = fsn

	for _, folder := range folderConfig {
		if folder == nil || f.pollerFor(folder.Path) != nil {
			continue
		}

		if err := fsn.Add(folder.Path); err != nil {
			logger.Errorf("Folder '%s' (cannot watch): %v", folder.Path, err)
		}
	}

	return nil
}

// newFolderPoller watches cfg.Path non-recursively, same as fsnotify.
// Existing nested folders are not listed until Folders.Add after a new item appears.
func newFolderPoller(cfg *FolderConfig) (*folderPoller, error) {
	pollWatcher := watcher.New()
	pollWatcher.FilterOps(watcher.Rename, watcher.Move, watcher.Write, watcher.Create, watcher.Remove)
	pollWatcher.IgnoreHiddenFiles(true)

	if err := pollWatcher.Add(cfg.Path); err != nil {
		return nil, fmt.Errorf("poller: %w", err)
	}

	return &folderPoller{
		path:     cfg.Path,
		interval: cfg.Interval.Duration,
		watcher:  pollWatcher,
	}, nil
}

// Close stops pollers and fsnotify. Safe for tests; the daemon does not call this.
func (f *Folders) Close() {
	if f == nil {
		return
	}

	for _, poller := range f.pollers {
		if poller != nil && poller.watcher != nil {
			poller.watcher.Close()
		}
	}

	if f.FSNotify != nil {
		_ = f.FSNotify.Close()
	}
}

// Add watches a nested path after a new archive or folder appears.
func (f *Folders) Add(folder string) error {
	if poller := f.pollerFor(folder); poller != nil {
		if err := poller.watcher.Add(folder); err != nil {
			return fmt.Errorf("poller: %w", err)
		}

		return nil
	}

	if f.FSNotify == nil {
		return nil
	}

	if err := f.FSNotify.Add(folder); err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}

	return nil
}

// Remove drops a nested watch when extract starts or the item goes away.
func (f *Folders) Remove(folder string) {
	if poller := f.pollerFor(folder); poller != nil {
		_ = poller.watcher.Remove(folder)
	}

	if f.FSNotify != nil {
		_ = f.FSNotify.Remove(folder)
	}
}

// StartPollers starts one radovskyb watcher per polled folder.
func (f *Folders) StartPollers() {
	for _, poller := range f.pollers {
		go f.startPoller(poller)
	}
}

func (f *Folders) startPoller(poller *folderPoller) {
	if err := poller.watcher.Start(poller.interval); err != nil {
		f.Errorf("folder poller stopped: %v", err)
	}
}

// PollerSummaries lists each poller as "path @ interval" for startup logs.
func (f *Folders) PollerSummaries() []string {
	out := make([]string, 0, len(f.pollers))
	for _, poller := range f.pollers {
		out = append(out, poller.path+" @ "+poller.interval.String())
	}

	return out
}

// FSNotifyPaths are watch roots that use filesystem events, not a poller.
func (f *Folders) FSNotifyPaths() []string {
	out := make([]string, 0, len(f.Config))
	for _, cfg := range f.Config {
		if cfg == nil || f.pollerFor(cfg.Path) != nil {
			continue
		}

		out = append(out, cfg.Path)
	}

	return out
}

func (f *Folders) pollerFor(path string) *folderPoller {
	cfg := f.watchConfig(path)
	if cfg == nil {
		return nil
	}

	for _, poller := range f.pollers {
		if poller != nil && poller.path == cfg.Path {
			return poller
		}
	}

	return nil
}

func (f *Folders) watchConfig(name string) *FolderConfig {
	name = filepath.Clean(name)

	var (
		best    *FolderConfig
		bestLen = -1
	)

	for _, cfg := range f.Config {
		if cfg == nil || cfg.Path == "" {
			continue
		}

		clean := filepath.Clean(cfg.Path)
		if !PathContains(clean, name) {
			continue
		}

		if len(clean) > bestLen {
			best = cfg
			bestLen = len(clean)
		}
	}

	return best
}

// WatchFSNotify reads file system events from a channel and processes them.
// This runs in its own go routine, and eventually sends the event back into the main routine.
func (f *Folders) WatchFSNotify() {
	defer log.Println("Folder watcher routine exited. No longer watching any folders.")

	var waitGroup sync.WaitGroup

	for _, poller := range f.pollers {
		waitGroup.Add(1)

		go func(poller *folderPoller) {
			defer waitGroup.Done()

			f.watchPoller(poller)
		}(poller)
	}

	if f.FSNotify != nil {
		f.readFSNotify()
	}

	waitGroup.Wait()
}

func (f *Folders) watchPoller(poller *folderPoller) {
	for {
		select {
		case err := <-poller.watcher.Error:
			f.Errorf("watcher: %v", err)
		case event := <-poller.watcher.Event:
			f.handleFileEvent(event.Path, "w "+event.Op.String())
		case <-poller.watcher.Closed:
			return
		}
	}
}

func (f *Folders) readFSNotify() {
	for {
		select {
		case err := <-f.FSNotify.Errors:
			f.Errorf("fsnotify: %v", err)
		case event, ok := <-f.FSNotify.Events:
			if !ok {
				return
			}

			f.handleFileEvent(event.Name, "f "+event.Op.String())
		}
	}
}

func (f *Folders) handleFileEvent(name, operation string) {
	if f.ignoredExtractName(name) {
		return
	}

	cfg := f.watchConfig(name)
	if cfg == nil {
		f.Debugf("Folder: Ignored event from non-configured path: %v", name)
		return
	}

	if filepath.Clean(name) == filepath.Clean(cfg.Path) {
		return
	}

	if cfg.IsExcludedPath(name) {
		f.Debugf("Folder: Ignored event from excluded path: %v", name)
		return
	}

	eventName := filepath.Base(name)
	if dir := filepath.Dir(name); filepath.Clean(dir) != filepath.Clean(cfg.Path) {
		eventName = filepath.Base(dir)
	}

	f.Events <- &Event{Name: eventName, Config: cfg, File: name, Op: operation}
}

// ProcessEvent processes the event that was received.
func (f *Folders) ProcessEvent(event *Event, now time.Time) {
	dirPath := filepath.Join(event.Config.Path, event.Name)

	if event.Config.IsExcludedPath(event.File) || event.Config.IsExcludedPath(dirPath) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (excluded path)", event.Op, event.File)
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

		f.Debugf("Folder: Ignored File Event (%s) '%s' (unreadable): %v", event.Op, event.File, err)

		return
	}

	if !stat.IsDir() && !xtractr.IsArchiveFile(event.Name) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (not archive or dir): %v", event.Op, event.File, err)
		return
	}

	if f.ignoredExtractName(dirPath) || f.ignoredExtractName(event.File) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (extract path)", event.Op, event.File)
		return
	}

	if stat.IsDir() && f.isExtractDest(dirPath) {
		f.Debugf("Folder: Ignored File Event (%s) '%s' (extract output)", event.Op, event.File)

		if _, ok := f.Folders[dirPath]; ok {
			f.Debugf("Folder: Removing Tracked Item: %v", dirPath)
			delete(f.Folders, dirPath)
			f.Remove(dirPath)
		}

		return
	}

	f.saveEvent(event, dirPath, now)
}

func (f *Folders) saveEvent(event *Event, dirPath string, now time.Time) {
	if _, ok := f.Folders[dirPath]; ok {
		f.Folders[dirPath].Updated = now
		return
	}

	if err := f.Add(dirPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			f.Errorf("Folder: Tracking New Item: %v (event: %s): %v ", dirPath, event.Op, err)
		}

		return
	}

	f.Printf("[Folder] Tracking New Item: %v (event: %s)", dirPath, event.Op)

	f.Folders[dirPath] = &Folder{
		Updated: now,
		Status:  extract.WAITING,
		Config:  event.Config,
	}
}

// ignoredExtractName is true when a path component is the temp extract folder
// (ends with IgnoreSuffix) or the extract log (_unpackerred.<archive>.txt).
func (f *Folders) ignoredExtractName(path string) bool {
	if f == nil || f.IgnoreSuffix == "" || path == "" {
		return false
	}

	for {
		base := filepath.Base(path)
		if strings.HasSuffix(base, f.IgnoreSuffix) || strings.HasPrefix(base, f.IgnoreSuffix+".") {
			return true
		}

		next := filepath.Dir(path)
		if next == path {
			return false
		}

		path = next
	}
}

// isExtractDest reports whether dir is xtractr output: it has the extract log,
// or it sits next to an archive whose stem matches the directory name
// (movie.iso → movie/ after the temp _unpackerred folder is renamed).
func (f *Folders) isExtractDest(dirPath string) bool {
	if hasExtractLog(dirPath, f.IgnoreSuffix) {
		return true
	}

	return hasSiblingArchiveStem(dirPath)
}

func hasExtractLog(dirPath, suffix string) bool {
	if suffix == "" {
		return false
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	prefix := suffix + "."

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(strings.ToLower(name), ".txt") {
			return true
		}
	}

	return false
}

func hasSiblingArchiveStem(dirPath string) bool {
	base := filepath.Base(dirPath)
	parent := filepath.Dir(dirPath)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if xtractr.IsArchiveFile(name) && archiveStem(name) == base {
			return true
		}
	}

	return false
}

// archiveStem strips archive extensions the same way xtractr names the final
// extract folder (twice, for tar.gz and friends).
func archiveStem(name string) string {
	stem := name

	for range 2 {
		if !xtractr.IsArchiveFile(stem) {
			break
		}

		next := strings.TrimSuffix(stem, filepath.Ext(stem))
		if next == stem {
			break
		}

		stem = next
	}

	return stem
}
