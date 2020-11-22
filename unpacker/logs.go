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

func (status ExtractStatus) MarshalText() ([]byte, error) {
	return []byte(status.Short()), nil
}

// MarshalText turns a status into a word, for a json identifier.
func (status ExtractStatus) Short() string {
	if status > DELETED {
		return "unknown"
	}

	return []string{
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
	}[status]
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
	var (
		waiting          uint
		queued           uint
		extracting       uint
		failed           uint
		extracted        uint
		imported         uint
		deleted          uint
		hookOK, hookFail = u.WebhookCounts()
	)

	for name := range u.Map {
		switch u.Map[name].Status {
		case WAITING:
			waiting++
		case QUEUED:
			queued++
		case EXTRACTING:
			extracting++
		case DELETEFAILED, EXTRACTFAILED:
			failed++
		case EXTRACTED:
			extracted++
		case DELETED, DELETING:
			deleted++
		case IMPORTED:
			imported++
		}
	}

	u.Logf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted]", waiting, queued, extracting, extracted, imported, failed, deleted)
	u.Logf("[Unpackerr] Totals: [%d restarted] [%d finished] [%d|%d webhooks] [%d stacks]",
		u.Restarted, u.Finished, hookOK, hookFail, len(u.folders.Events)+len(u.updates)+len(u.folders.Updates))
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
