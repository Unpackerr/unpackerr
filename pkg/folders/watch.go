package folders

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/fsnotify/fsnotify"
	"github.com/radovskyb/watcher"
	"golift.io/xtractr"
)

// NewWatcher returns a new folder watcher.
// You must call folders.FSNotify.Close() when you're done with it.
func (c WatchConfig) NewWatcher(
	folderConfig []*FolderConfig,
	logger Logs,
	updateBuf int,
	ignoreSuffix string,
) (*Folders, error) {
	folders := &Folders{
		Interval:     c.Interval.Duration,
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
			logger.Errorf("Folder '%s' (cannot poll): %v", folder.Path, err)
		}

		if err := fsn.Add(folder.Path); err != nil {
			logger.Errorf("Folder '%s' (cannot watch): %v", folder.Path, err)
		}
	}

	return folders, nil
}

// Add uses either fsnotify or watcher.
func (f *Folders) Add(folder string) error {
	if f.Interval >= MinimumPollInterval {
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

// StartPoller starts the radovskyb poll watcher.
func (f *Folders) StartPoller(interval time.Duration) error {
	if err := f.Watcher.Start(interval); err != nil {
		return fmt.Errorf("folder poller stopped: %w", err)
	}

	return nil
}

// WatchFSNotify reads file system events from a channel and processes them.
// This runs in its own go routine, and eventually sends the event back into the main routine.
func (f *Folders) WatchFSNotify() {
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

func (f *Folders) handleFileEvent(name, operation string) {
	if f.ignoredExtractName(name) {
		return
	}

	for _, cfg := range f.Config {
		// Do not handle events on the watched folder itself.
		if name == cfg.Path {
			return
		}

		if !strings.HasPrefix(name, cfg.Path) {
			continue // Not the configured folder for the event we just got.
		}

		if cfg.IsExcludedPath(name) {
			f.Debugf("Folder: Ignored event from excluded path: %v", name)
			continue
		}

		if dir := filepath.Dir(name); dir == cfg.Path {
			f.Events <- &Event{Name: filepath.Base(name), Config: cfg, File: name, Op: operation}
		} else {
			f.Events <- &Event{Name: filepath.Base(dir), Config: cfg, File: name, Op: operation}
		}

		return
	}

	f.Debugf("Folder: Ignored event from non-configured path: %v", name)
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
