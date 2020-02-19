package unpacker

import (
	"fmt"
	"log"
	"os"
)

// DeLogf writes Debug log lines.
func (u *Unpackerr) DeLogf(msg string, v ...interface{}) {
	const callDepth = 2

	if u.Config.Debug {
		_ = log.Output(callDepth, fmt.Sprintf("[DEBUG] "+msg, v...))
	}
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
