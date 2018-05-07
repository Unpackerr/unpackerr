package main

import (
	"log"
	"os"
	"time"

	unrar "github.com/jagadeesh-kotra/gorar"
)

/*
  Extracts refers the transfers identified as completed and now eligible for
  decompression. Only completed transfers that have a .rar file will end up
  "with a status."
*/

// CreateStatus for a newly-started extraction. It will also overwrite.
func (r *RunningData) CreateStatus(name, path, file, app string, status ExtractStatus) {
	r.hisS.Lock()
	defer r.hisS.Unlock()
	r.History[name] = Extracts{
		RARFile:  file,
		BasePath: path,
		App:      app,
		Status:   status,
		Updated:  time.Now(),
	}
}

// GetHistory returns a copy of the extracts map.
func (r *RunningData) GetHistory() map[string]Extracts {
	r.hisS.RLock()
	defer r.hisS.RUnlock()
	return r.History
}

// GetStatus returns the status history for an extraction.
func (r *RunningData) GetStatus(name string) (e Extracts) {
	if data, ok := r.GetHistory()[name]; ok {
		e = data
	}
	return
}

// UpdateStatus for an on-going tracked extraction.
func (r *RunningData) UpdateStatus(name string, status ExtractStatus, fileList []string) {
	r.hisS.Lock()
	defer r.hisS.Unlock()
	if _, ok := r.History[name]; !ok {
		// .. this only happens if you mess up in the code.
		log.Println("ERROR: Unable to update missing History for", name)
		return
	}
	h := Extracts{
		RARFile:  r.History[name].RARFile,
		BasePath: r.History[name].BasePath,
		App:      r.History[name].App,
		FileList: r.History[name].FileList,
		Status:   status,
		Updated:  time.Now(),
	}
	if fileList != nil {
		h.FileList = fileList
	}
	r.History[name] = h
}

// Extracts a rar archive with history updates, and some meta data display.
func (r *RunningData) extractFile(name, path, file string) {
	log.Println("Extraction Queued:", file)
	r.rarS.Lock() // One extraction at a time.
	defer r.rarS.Unlock()
	log.Println("Extracting:", file)
	r.UpdateStatus(name, EXTRACTING, nil)
	files := getFileList(path) // get the "before extraction" file list
	start := time.Now()
	if err := unrar.RarExtractor(file, path); err != nil {
		log.Printf("Extraction Error: %v to %v (elapsed %v): %v", file, path, time.Now().Sub(start).Round(time.Second), err)
		r.UpdateStatus(name, EXTRACTFAILED, nil)
	} else {
		r.UpdateStatus(name, EXTRACTED, difference(files, getFileList(path)))
		log.Printf("Extracted: %v (%d files, elapsed %v)", file, len(r.GetStatus(name).FileList), time.Now().Sub(start).Round(time.Second))
	}
}

// Deletes extracted files after Sonarr/Radarr imports them.
func (r *RunningData) deleteFiles(name string, files []string) {
	status := DELETED
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Println("Delete Error:", file)
			status = DELFAILED
			// TODO: clean this up another way? It just goes stale like this.
			continue
		}
		log.Println("Deleted:", file)
	}
	r.UpdateStatus(name, status, nil)
}
