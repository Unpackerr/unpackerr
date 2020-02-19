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

	if u.Config.Debug {
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

		u.Map[data.Path].Status = data.Status
		u.Map[data.Path].Files = append(u.Map[data.Path].Files, data.Files...)
	} else {
		// This is a new folder being extracted.
		u.Map[data.Path] = data
		u.Map[data.Path].Status = QUEUED
	}

	u.Map[data.Path].Updated = time.Now()
}

// printCurrentQueue returns the number of things happening.
func (u *Unpackerr) printCurrentQueue() {
	e := eCounters{}

	for name := range u.Map {
		switch u.Map[name].Status {
		case DOWNLOADING:
			e.waiting++
		case QUEUED:
			e.queued++
		case EXTRACTING:
			e.extracting++
		case DELETEFAILED, EXTRACTFAILED:
			e.failed++
		case EXTRACTED:
			e.extracted++
		case DELETED, DELETING:
			e.deleted++
		case IMPORTED:
			e.imported++
		}
	}

	log.Printf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted], Totals: [%d restarts] [%d finished]",
		e.waiting, e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
		u.Restarted, u.Finished)
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
