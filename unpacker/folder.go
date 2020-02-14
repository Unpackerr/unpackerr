package unpacker

/* Folder Watching Codez */

import (
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Folders holds all known (created) folders in all watch paths.
type Folders struct {
	Config  []*folderConfig
	Folders map[string]*Folder
	NewChan chan *eventData
	Updates chan *update
	DeLogf  func(msg string, v ...interface{})
	Extract func(name, app, path string, moveBack bool)
	*fsnotify.Watcher
}

// Folder is a "new" watched folder.
type Folder struct {
	last time.Time
	step ExtractStatus
	cnfg *folderConfig
	list []string
}

type eventData struct {
	cnfg *folderConfig
	name string
	file string
}

type update struct {
	Step     ExtractStatus
	Name     string
	Extracts *Extracts
}

const (
	// provide a little splay between timers.
	splay = 3 * time.Second
	// suffix for unpacked folders.
	suffix = "_unpackerred"
)

// PollFolders begins the routines to watch folders for changes.
// if those changes include the addition of compressed files, they
// are processed for exctraction.
func (u *Unpackerr) PollFolders() {
	var flist []string

	var err error

	if u.Config.Folders, flist = u.checkFolders(); len(u.Config.Folders) < 1 {
		u.DeLogf("[FOLDER] Nothing to watch, or none folders configured.")
		return
	}

	time.Sleep(splay)
	log.Println("[FOLDER] Watching:", strings.Join(flist, ", "))

	u.folders, err = u.NewFolderWatcher()
	if err != nil {
		log.Println("[ERROR] Watching Folders:", err)
	}
	defer u.folders.Close()

	go u.folders.Track()
	u.folders.Watch()
	log.Println("[ERROR] No longer watching folders!")
}

// checkFolders stats all configured folders and returns only "good" ones.
func (u *Unpackerr) checkFolders() ([]*folderConfig, []string) {
	goodFolders := []*folderConfig{}
	goodFlist := []string{}

	for _, f := range u.Folders {
		if stat, err := os.Stat(f.Path); err != nil {
			log.Println("[ERROR] Folder (cannot watch):", err)
			continue
		} else if !stat.IsDir() {
			log.Printf("[ERROR] Folder (cannot watch): %s: not a folder", f.Path)
			continue
		}

		f.Path = strings.TrimSuffix(f.Path, "/") + "/"
		goodFolders = append(goodFolders, f)
		goodFlist = append(goodFlist, f.Path)
	}

	return goodFolders, goodFlist
}

// NewFolderWatcher returns a new folder watcher.
// You must call folders.Watcher.Close() when you're done with it.
func (u *Unpackerr) NewFolderWatcher() (*Folders, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, folder := range u.Folders {
		if err := watcher.Add(folder.Path); err != nil {
			log.Println("[ERROR] Folder (cannot watch):", err)
		}
	}

	return &Folders{
		Config:  u.Folders,
		Folders: make(map[string]*Folder),
		NewChan: make(chan *eventData, 10000),
		Updates: make(chan *update, 100),
		DeLogf:  u.DeLogf,
		Watcher: watcher,
		Extract: u.HandleCompleted,
	}, nil
}

// Watch keeps an eye on the tracked folders.
func (f *Folders) Watch() {
	for {
		select {
		case err, ok := <-f.Errors:
			if !ok {
				return
			}

			log.Println("[ERROR] fsnotify:", err)
		case event, ok := <-f.Events:
			if !ok {
				return
			}

			f.Event(event)
		}
	}
}

// Event turns a raw watched-folder event into an internal event and sends it off.
func (f *Folders) Event(event fsnotify.Event) {
	for _, cnfg := range f.Config {
		// Find the configured folder for the event we just got.
		if !strings.HasPrefix(event.Name, cnfg.Path) {
			continue
		}

		// folder.Path: "/Users/Documents/auto"
		// event.Name: "/Users/Documents/auto/my_folder/file.rar"
		// name: "my_folder"
		p := strings.TrimPrefix(event.Name, cnfg.Path)
		if np := path.Dir(p); np != "." {
			p = np
		}

		if strings.HasSuffix(p, suffix) {
			// it's our item, ignore it.
			return
		}

		f.NewChan <- &eventData{name: p, cnfg: cnfg, file: path.Base(event.Name)}

		return
	}
}

// Track keeps track of things being updated and extracted.
func (f *Folders) Track() {
	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case event, ok := <-f.NewChan:
			if !ok {
				return
			}

			f.processEvent(event)
		case <-ticker.C:
			f.checkForWork() // Look for things to do every minute.
		case update := <-f.Updates:
			f.processUpdate(update) // process extract update
		}
	}
}

// Delete cleans up a folder after it has reached the end of its delete timer.
func (f *Folders) Delete(name string) {
	folder, ok := f.Folders[name]
	if !ok {
		return
	}

	delete(f.Folders, name)

	if folder.cnfg.DeleteOrig {
		f.DeLogf("Deleting:", folder.cnfg.Path)

		if err := os.RemoveAll(folder.cnfg.Path); err != nil {
			log.Println("[ERROR] Deleting:", err)
		}
	}

	if folder.cnfg.MoveBack {
		return
	}

	f.DeLogf("Deleting:", folder.cnfg.Path+suffix)

	if err := os.RemoveAll(folder.cnfg.Path + suffix); err != nil {
		log.Println("[ERROR] Deleting:", err)
	}
}

// processEvent processes the event that was received.
func (f *Folders) processEvent(event *eventData) {
	fullPath := path.Join(event.cnfg.Path, event.name)
	if stat, err := os.Stat(fullPath); err != nil {
		// Item is unusable (probably deleted), remove it from history.
		f.DeLogf("Removing Tracked Item: %v", fullPath)
		delete(f.Folders, fullPath)
		_ = f.Watcher.Remove(fullPath)

		return
	} else if !stat.IsDir() {
		//		f.DeLogf("Ignoring Item: %v (not a folder)", fullPath)
		return
	}

	if _, ok := f.Folders[fullPath]; ok {
		//		f.DeLogf("Item Updated: %v (file: %v)", fullPath, event.file)
		f.Folders[fullPath].last = time.Now()
		return
	}

	if err := f.Watcher.Add(fullPath); err != nil {
		log.Printf("[ERROR] Tracking New Item: %v: %v", fullPath, err)
		return
	}

	log.Printf("[FOLDER] Tracking New Item: %v", fullPath)

	f.Folders[fullPath] = &Folder{
		last: time.Now(),
		step: MISSING,
		cnfg: event.cnfg,
	}
}

// checkForWork runs at an interval to see if any folders are ready for extraction.
func (f *Folders) checkForWork() {
	for name, folder := range f.Folders {
		if time.Since(folder.last) > folder.cnfg.DeleteAfter.Duration && folder.step == EXTRACTED {
			// go delete it.
			f.Delete(name)
		}
		// If the folder was written to in the last two minutes, skip it, it's not ready.
		// Or, if it's already begun (or finished) extraction, skip it.
		if time.Since(folder.last) < 2*time.Minute || folder.step != MISSING {
			continue
		}

		// extract it.
		f.Folders[name].step = QUEUED
		f.Folders[name].last = time.Now()
		_ = f.Watcher.Remove(name)

		// This method belongs to the extraction code.
		go f.Extract(folder.cnfg.Path, "Folder", name, folder.cnfg.MoveBack)
	}
}

func (f *Folders) processUpdate(u *update) {
	if _, ok := f.Folders[u.Name]; !ok {
		return
	}

	f.Folders[u.Name].last = time.Now()
	f.Folders[u.Name].step = u.Step

	if u.Extracts != nil {
		f.extractCallback(u.Extracts, u.Name)
	}
}

// extractCallback is the callback from the extraction code.
func (f *Folders) extractCallback(data *Extracts, name string) {
	folder, ok := f.Folders[name]
	if !ok {
		log.Printf("[FOLDER] Extract Finished, folder missing, nothing else to do: %s (files extracted: %d)",
			name, len(data.Files))
		return // this likely can't happen.
	}

	if folder.cnfg.DeleteAfter.Duration == 0 {
		delete(f.Folders, name)
		return
	}

	// This folder stays in history until it's deleted.
	f.Folders[name].list = data.Files
}

// handleFolder is the initial callback from a completed extraction.
// This updates the global state, and then passes the update into the callback.
func (u *Unpackerr) handleFolder(data *Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()
	delete(u.History.Map, name)
	u.Finished++
	// Send the update back into our channel (single go routine).
	u.folders.Updates <- &update{Step: data.Status, Extracts: data, Name: name}
}
