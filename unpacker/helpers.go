package unpacker

import (
	"fmt"
	"log"
	"os"
	"time"
)

// DeLogf writes Debug log lines.
func (u *Unpackerr) DeLogf(msg string, v ...interface{}) {
	const callDepth = 2

	if u.Debug {
		_ = log.Output(callDepth, fmt.Sprintf("[DEBUG] "+msg, v...))
	}
}

// updateQueueStatus for an on-going tracked extraction.
// This is called from a channel callback to update status in a single go routine.
func (u *Unpackerr) updateQueueStatus(data *Extracts) {
	if _, ok := u.Map[data.Path]; ok {
		if data.Status == DELETED {
			// This is a completed folder.
			u.Finished++
			delete(u.Map, data.Path)

			return
		}

		u.Map[data.Path] = &Extracts{
			Status: data.Status,
			Files:  append(u.Map[data.Path].Files, data.Files...),
		}
	} else {
		// This is a new folder being extracted.
		u.Map[data.Path] = data
		u.Map[data.Path].Status = QUEUED
	}

	u.Map[data.Path].Updated = time.Now()
}

// eCount returns the number of things happening.
func (u *Unpackerr) eCount(e *eCounters, status ExtractStatus) {
	switch status {
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

// DeleteFiles obliterates things and logs. Use with caution.
func DeleteFiles(files ...string) {
	for _, file := range files {
		if err := os.RemoveAll(file); err != nil {
			log.Printf("Error: Deleting %v: %v", file, err)
			continue
		}

		log.Printf("Deleted (recursively): %s", file)
	}
}

// custom percentage procedure for *arr apps.
func percent(size, total float64) int {
	const oneHundred = 100
	return int(oneHundred - (size / total * oneHundred))
}
