package main

import (
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	unrar "github.com/jagadeesh-kotra/gorar"
)

/*
  Extracts refers the transfers identified as completed and now eligible for
  decompression. Only completed transfers that have a .rar file will end up
  "with a status."
*/

// CreateStatus for a newly-started extraction. It will also overwrite.
func (r *RunningData) CreateStatus(name, path string, app string, files []string) {
	r.hisS.Lock()
	defer r.hisS.Unlock()
	r.History[name] = Extracts{
		Path:    path,
		App:     app,
		Status:  QUEUED,
		Updated: time.Now(),
	}
}

// GetHistory returns a copy of the extracts map.
func (r *RunningData) GetHistory() map[string]Extracts {
	r.hisS.RLock()
	defer r.hisS.RUnlock()
	return r.History
}

// DeleteStatus deletes a deleted item from internal history.
func (r *RunningData) DeleteStatus(name string) {
	r.hisS.RLock()
	defer r.hisS.RUnlock()
	delete(r.History, name)
}

// GetStatus returns the status history for an extraction.
func (r *RunningData) GetStatus(name string) (e Extracts) {
	if data, ok := r.GetHistory()[name]; ok {
		e = data
	}
	return
}

// eCount returns the number of things happening.
func (r *RunningData) eCount() (e eCounters) {
	r.hisS.RLock()
	defer r.hisS.RUnlock()
	for _, r := range r.History {
		switch r.Status {
		case QUEUED:
			e.queued++
		case EXTRACTING:
			e.extracting++
		case DELETEFAILED, EXTRACTFAILED, EXTRACTFAILED2:
			e.failed++
		case EXTRACTED:
			e.extracted++
		case DELETED, DELETING:
			e.deleted++
		case IMPORTED:
			e.imported++
		}
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
	r.History[name] = Extracts{
		Path:    r.History[name].Path,
		App:     r.History[name].App,
		Files:   append(r.History[name].Files, fileList...),
		Status:  status,
		Updated: time.Now(),
	}
}

// Count the extracts, check if too many are active, then grant or deny another.
func (r *RunningData) extractMayProceed(name string) bool {
	r.hisS.Lock()
	defer r.hisS.Unlock()
	if r.History[name].Updated.Add(time.Minute).After(time.Now()) {
		// Item must be queued for at least 1 minute to prevent Deluge races.
		return false
	}
	var count int
	for _, r := range r.History {
		if r.Status == EXTRACTING {
			count++
		}
	}
	if count < r.maxExtracts {
		r.History[name] = Extracts{
			Path:    r.History[name].Path,
			App:     r.History[name].App,
			Files:   r.History[name].Files,
			Status:  EXTRACTING,
			Updated: time.Now(),
		}
		return true
	}
	return false
}

// Extracts rar archives with history updates, and some meta data display.
func (r *RunningData) extractFiles(name, path string, archives []string) {
	if len(archives) == 1 {
		log.Printf("Extract Enqueued: (1 file) - %v", name)
	} else {
		log.Printf("Extract Group Enqueued: %d file(s) - %v", len(archives), name)
	}
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	// This works because extractMayProceed has a lock on the checking and setting of the value.
	for !r.extractMayProceed(name) {
		time.Sleep(time.Duration(rand.Float64()) * time.Second)
	}

	log.Printf("Extract Starting (%d active): %d file(s) - %v", r.eCount().extracting, len(archives), name)
	// Extract into a temporary path so Sonarr doesn't import episodes prematurely.
	tmpPath := path + "_unpacker"
	if err := os.MkdirAll(tmpPath, 0755); err != nil {
		log.Println("Extract Error: Creating temporary extract folder:", err.Error())
		r.UpdateStatus(name, EXTRACTFAILED, nil)
		return
	}

	start := time.Now()
	extras := 0

	// Extract one archive at a time, then check if it contained any more archives.
	for i, file := range archives {
		fileStart := time.Now()
		beforeFiles := getFileList(tmpPath) // get the "before this extraction" file list
		if err := unrar.RarExtractor(file, tmpPath); err != nil {
			log.Printf("Extract Error: [%d/%d] %v to %v (%v elapsed): %v",
				i+1, len(archives), file, tmpPath, time.Now().Sub(fileStart).Round(time.Second), err)
			r.UpdateStatus(name, EXTRACTFAILED, getFileList(tmpPath))
			return
		}

		newFiles := difference(beforeFiles, getFileList(tmpPath))
		log.Printf("Extract Complete: [%d/%d] %v (%v elapsed, %d files)",
			i+1, len(archives), file, time.Now().Sub(fileStart).Round(time.Second), len(newFiles))

		// Check if we just extracted more archives.
		for _, file := range newFiles {
			// Do this now, instead of re-queuing, so subs are imported.
			if strings.HasSuffix(file, ".rar") {
				log.Printf("Extracted RAR Archive, Extracting Additional File: %v", file)
				if err := unrar.RarExtractor(file, tmpPath); err != nil {
					log.Printf("Extract Error: [%d/%d](extra) %v to %v (%v elapsed): %v",
						i+1, len(archives), file, tmpPath, time.Now().Sub(fileStart).Round(time.Second), err)
					r.UpdateStatus(name, EXTRACTFAILED, getFileList(tmpPath))
					return
				}
				log.Printf("Extract Complete: [%d/%d](extra) %v (%v elapsed)",
					i+1, len(archives), file, time.Now().Sub(fileStart).Round(time.Second))
				extras++
			}
		}
	}

	// Move the extracted files back into their original folder.
	newFiles, err := moveFiles(tmpPath, path)
	if err != nil {
		log.Printf("Extract Rename Error: %v (%d+%d archives, %d files, %v elapsed): %v",
			name, len(archives), extras, len(newFiles), time.Now().Sub(start).Round(time.Second), err.Error())
		r.UpdateStatus(name, EXTRACTFAILED, newFiles)
		return
	}

	log.Printf("Extract Group Complete: %v (%d+%d archives, %d files, %v elapsed)",
		name, len(archives), extras, len(newFiles), time.Now().Sub(start).Round(time.Second))
	r.UpdateStatus(name, EXTRACTED, newFiles)
}
