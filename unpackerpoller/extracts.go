package unpackerpoller

import (
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	rar "golift.io/rar"
)

/*
  Extracts refers the transfers identified as completed and now eligible for
  decompression. Only completed transfers that have a .rar file will end up
  "with a status."
*/

// CreateStatus for a newly-started extraction. It will also overwrite.
func (u *UnpackerPoller) CreateStatus(name, path string, app string, files []string) {
	u.History.Lock()
	defer u.History.Unlock()
	u.History.Map[name] = Extracts{
		Path:    path,
		App:     app,
		Status:  QUEUED,
		Updated: time.Now(),
	}
}

// GetHistory returns a copy of the extracts map.
func (u *UnpackerPoller) GetHistory() map[string]Extracts {
	u.History.RLock()
	defer u.History.RUnlock()
	return u.History.Map
}

// DeleteStatus deletes a deleted item from internal history.
func (u *UnpackerPoller) DeleteStatus(name string) {
	u.History.RLock()
	defer u.History.RUnlock()
	delete(u.History.Map, name)
}

// GetStatus returns the status history for an extraction.
func (u *UnpackerPoller) GetStatus(name string) (e Extracts) {
	if data, ok := u.GetHistory()[name]; ok {
		e = data
	}
	return
}

// eCount returns the number of things happening.
func (u *UnpackerPoller) eCount() (e eCounters) {
	u.History.RLock()
	defer u.History.RUnlock()
	for _, r := range u.History.Map {
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
func (u *UnpackerPoller) UpdateStatus(name string, status ExtractStatus, fileList []string) {
	u.History.Lock()
	defer u.History.Unlock()
	if _, ok := u.History.Map[name]; !ok {
		// .. this only happens if you mess up in the code.
		log.Println("ERROR: Unable to update missing History for", name)
		return
	}
	u.History.Map[name] = Extracts{
		Path:    u.History.Map[name].Path,
		App:     u.History.Map[name].App,
		Files:   append(u.History.Map[name].Files, fileList...),
		Status:  status,
		Updated: time.Now(),
	}
}

// Count the extracts, check if too many are active, then grant or deny another.
func (u *UnpackerPoller) extractMayProceed(name string) bool {
	u.History.Lock()
	defer u.History.Unlock()
	if u.History.Map[name].Updated.Add(time.Minute).After(time.Now()) {
		// Item must be queued for at least 1 minute to prevent Deluge races.
		return false
	}
	var count int
	for _, r := range u.History.Map {
		if r.Status == EXTRACTING {
			count++
		}
	}
	if count < u.ConcurrentExtracts {
		u.History.Map[name] = Extracts{
			Path:    u.History.Map[name].Path,
			App:     u.History.Map[name].App,
			Files:   u.History.Map[name].Files,
			Status:  EXTRACTING,
			Updated: time.Now(),
		}
		return true
	}
	return false
}

// Extracts rar archives with history updates, and some meta data display.
func (u *UnpackerPoller) extractFiles(name, path string, archives []string) {
	if len(archives) == 1 {
		log.Printf("Extract Enqueued: (1 file) - %v", name)
	} else {
		log.Printf("Extract Group Enqueued: %d file(s) - %v", len(archives), name)
	}
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	// This works because extractMayProceed has a lock on the checking and setting of the value.
	for !u.extractMayProceed(name) {
		time.Sleep(time.Duration(rand.Float64()) * time.Second)
	}

	log.Printf("Extract Starting (%d active): %d file(s) - %v", u.eCount().extracting, len(archives), name)
	// Extract into a temporary path so Sonarr doesn't import episodes prematurely.
	tmpPath := path + "_unpacker"
	if err := os.MkdirAll(tmpPath, 0755); err != nil {
		log.Println("Extract Error: Creating temporary extract folder:", err.Error())
		u.UpdateStatus(name, EXTRACTFAILED, nil)
		return
	}

	start := time.Now()
	extras := 0

	// Extract one archive at a time, then check if it contained any more archives.
	for i, file := range archives {
		fileStart := time.Now()
		beforeFiles := getFileList(tmpPath) // get the "before this extraction" file list
		if err := rar.RarExtractor(file, tmpPath); err != nil {
			log.Printf("Extract Error: [%d/%d] %v to %v (%v elapsed): %v",
				i+1, len(archives), file, tmpPath, time.Since(fileStart).Round(time.Second), err)
			u.UpdateStatus(name, EXTRACTFAILED, getFileList(tmpPath))
			return
		}

		newFiles := difference(beforeFiles, getFileList(tmpPath))
		log.Printf("Extract Complete: [%d/%d] %v (%v elapsed, %d files)",
			i+1, len(archives), file, time.Since(fileStart).Round(time.Second), len(newFiles))

		// Check if we just extracted more archives.
		for _, file := range newFiles {
			// Do this now, instead of re-queuing, so subs are imported.
			if strings.HasSuffix(file, ".rar") {
				log.Printf("Extracted RAR Archive, Extracting Additional File: %v", file)
				if err := rar.RarExtractor(file, tmpPath); err != nil {
					log.Printf("Extract Error: [%d/%d](extra) %v to %v (%v elapsed): %v",
						i+1, len(archives), file, tmpPath, time.Since(fileStart).Round(time.Second), err)
					u.UpdateStatus(name, EXTRACTFAILED, getFileList(tmpPath))
					return
				}
				log.Printf("Extract Complete: [%d/%d](extra) %v (%v elapsed)",
					i+1, len(archives), file, time.Since(fileStart).Round(time.Second))
				extras++
			}
		}
	}

	// Move the extracted files back into their original folder.
	newFiles, err := u.moveFiles(tmpPath, path)
	if err != nil {
		log.Printf("Extract Rename Error: %v (%d+%d archives, %d files, %v elapsed): %v",
			name, len(archives), extras, len(newFiles), time.Since(start).Round(time.Second), err.Error())
		u.UpdateStatus(name, EXTRACTFAILED, newFiles)
		return
	}

	log.Printf("Extract Group Complete: %v (%d+%d archives, %d files, %v elapsed)",
		name, len(archives), extras, len(newFiles), time.Since(start).Round(time.Second))
	u.UpdateStatus(name, EXTRACTED, newFiles)
}
