package unpacker

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

const callDepth = 2 // satisfy gomnd

// Debug writes Debug log lines... to stdout and/or a file.
func (u *Unpackerr) Debug(msg string, v ...interface{}) {
	if u.Config.Debug {
		_ = u.log.Output(callDepth, "[DEBUG] "+fmt.Sprintln(v...))
	}
}

// Log writes log lines... to stdout and/or a file.
func (u *Unpackerr) Log(v ...interface{}) {
	_ = u.log.Output(callDepth, fmt.Sprintln(v...))
}

// Logf writes log lines... to stdout and/or a file.
func (u *Unpackerr) Logf(msg string, v ...interface{}) {
	_ = u.log.Output(callDepth, fmt.Sprintf(msg, v...))
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() error {
	var write []io.Writer

	if u.log.SetFlags(log.LstdFlags); u.Config.Debug {
		u.log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	if !u.Config.Quiet {
		write = append(write, os.Stdout)
	}

	if u.Config.LogFile != "" {
		f, err := os.OpenFile(u.Config.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil {
			return err
		}

		write = append(write, f)
	}

	if len(write) == 0 {
		u.log.SetOutput(ioutil.Discard) // default is "nothing"
		return nil
	}

	in, out := io.Pipe()
	u.log.SetOutput(out)

	go func() {
		_, err := io.Copy(io.MultiWriter(write...), in)
		log.Fatalln("[ERROR] Logging Error:", err)
	}()

	return nil
}

// printCurrentQueue returns the number of things happening.
func (u *Unpackerr) printCurrentQueue() {
	e := eCounters{}

	for name := range u.Map {
		switch u.Map[name].Status {
		case WAITING:
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

	u.Logf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted], Totals: [%d restarts] [%d finished]",
		e.waiting, e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
		u.Restarted, u.Finished)
}

// DeleteFiles obliterates things and logs. Use with caution.
func (u *Unpackerr) DeleteFiles(files ...string) {
	for _, file := range files {
		if err := os.RemoveAll(file); err != nil {
			u.Logf("Error: Deleting %v: %v", file, err)
			continue
		}

		u.Logf("Deleted (recursively): %s", file)
	}
}

// custom percentage procedure for *arr apps.
func percent(size, total float64) int {
	const oneHundred = 100
	return int(oneHundred - (size / total * oneHundred))
}
