package unpacker

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

// satisfy gomnd.
const callDepth = 2 // log the line that called us.

// Use in r.eCount to return activity counters.
type eCounters struct {
	waiting    uint
	queued     uint
	extracting uint
	failed     uint
	extracted  uint
	imported   uint
	deleted    uint
	hookOK     uint
	hookFail   uint
}

// ExtractStatus is our enum for an extract's status.
type ExtractStatus uint8

// Extract Statuses.
const (
	WAITING = ExtractStatus(iota)
	QUEUED
	EXTRACTING
	EXTRACTFAILED
	EXTRACTED
	IMPORTED
	DELETING
	DELETEFAILED // unused
	DELETED
)

// String makes ExtractStatus human readable.
func (status ExtractStatus) String() string {
	if status > DELETED {
		return "Unknown"
	}

	return []string{
		// The order must not be be faulty.
		"Waiting, pre-Queue",
		"Queued",
		"Extracting",
		"Extraction Failed",
		"Extracted, Awaiting Import",
		"Imported",
		"Deleting",
		"Delete Failed",
		"Deleted",
	}[status]
}

// MarshalText turns a status into a word, for a json identifier.
func (status ExtractStatus) MarshalText() ([]byte, error) {
	if status > DELETED {
		return []byte("unknown"), nil
	}

	return []byte([]string{
		// The order must not be be faulty.
		"waiting",
		"queued",
		"extracting",
		"extractfailed",
		"extracted",
		"imported",
		"deleting",
		"deletefailed",
		"deleted",
	}[status]), nil
}

// Debug writes Debug log lines... to stdout and/or a file.
func (u *Unpackerr) Debug(msg string, v ...interface{}) {
	if u.Config.Debug {
		_ = u.log.Output(callDepth, "[DEBUG] "+fmt.Sprintf(msg, v...))
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

// logCurrentQueue prints the number of things happening.
func (u *Unpackerr) logCurrentQueue() {
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

	for _, hook := range u.Webhook {
		e.hookOK += hook.posts
		e.hookFail += hook.fails
	}

	u.Logf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted]", e.waiting, e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
	)
	u.Logf("[Unpackerr] Totals: [%d restarted] [%d finished] [%d|%d webhooks]",
		u.Restarted, u.Finished, e.hookOK, e.hookFail)
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() error {
	var writeFile io.Writer

	if u.log.SetFlags(log.LstdFlags); u.Config.Debug {
		u.log.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	if u.Config.LogFile != "" {
		f, err := os.OpenFile(u.Config.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil {
			return fmt.Errorf("os.OpenFile: %w", err)
		}

		writeFile = f
	}

	switch { // only use MultiWriter is we have > 1 writer.
	case !u.Config.Quiet && writeFile != nil:
		u.log.SetOutput(io.MultiWriter(writeFile, os.Stdout))
	case !u.Config.Quiet && writeFile == nil:
		u.log.SetOutput(os.Stdout)
	case writeFile == nil:
		u.log.SetOutput(ioutil.Discard) // default is "nothing"
	default:
		u.log.SetOutput(writeFile)
	}

	return nil
}

// logStartupInfo prints info about our startup config.
func (u *Unpackerr) logStartupInfo() {
	u.Log("==> Startup Settings <==")
	u.logSonarr()
	u.logRadarr()
	u.logLidarr()
	u.logReadarr()
	u.logFolders()
	u.Log(" => Parallel:", u.Config.Parallel)
	u.Log(" => Interval:", u.Config.Interval.Duration)
	u.Log(" => Delete Delay:", u.Config.DeleteDelay.Duration)
	u.Log(" => Start Delay:", u.Config.StartDelay.Duration)
	u.Log(" => Retry Delay:", u.Config.RetryDelay.Duration)
	u.Log(" => Debug / Quiet:", u.Config.Debug, "/", u.Config.Quiet)
	u.Log(" => Directory & File Modes:", u.Config.DirMode, "&", u.Config.FileMode)

	if u.Config.LogFile != "" {
		u.Log(" => Log File:", u.Config.LogFile)
	}

	u.logWebhook()
}
