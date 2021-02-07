package unpackerr

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	"golift.io/rotatorr"
	"golift.io/rotatorr/timerotator"
)

// satisfy gomnd.
const (
	callDepth   = 2 // log the line that called us.
	megabyte    = 1024 * 1024
	logsDirMode = 0755
)

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

// Desc makes ExtractStatus human readable.
func (status ExtractStatus) Desc() string {
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
	return []byte(status.String()), nil
}

// String turns a status into a short string.
func (status ExtractStatus) String() string {
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

// Debugf writes Debug log lines... to stdout and/or a file.
func (l *Logger) Debugf(msg string, v ...interface{}) {
	if l.debug {
		err := l.Logger.Output(callDepth, "[DEBUG] "+fmt.Sprintf(msg, v...))
		if err != nil {
			fmt.Println("Logger Error:", err) //nolint:forbidigo
		}
	}
}

// Print writes log lines... to stdout and/or a file.
func (l *Logger) Print(v ...interface{}) {
	err := l.Logger.Output(callDepth, fmt.Sprintln(v...))
	if err != nil {
		fmt.Println("Logger Error:", err) //nolint:forbidigo
	}
}

// Printf writes log lines... to stdout and/or a file.
func (l *Logger) Printf(msg string, v ...interface{}) {
	err := l.Logger.Output(callDepth, fmt.Sprintf(msg, v...))
	if err != nil {
		fmt.Println("Logger Error:", err) //nolint:forbidigo
	}
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

	u.Printf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted]", waiting, queued, extracting, extracted, imported, failed, deleted)
	u.Printf("[Unpackerr] Totals: [%d retries] [%d finished] [%d|%d webhooks] [%d stacks]",
		u.Retries, u.Finished, hookOK, hookFail, len(u.folders.Events)+len(u.updates)+len(u.folders.Updates))
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() {
	u.Logger.debug = u.Config.Debug

	if u.Logger.Logger.SetFlags(log.LstdFlags); u.Config.Debug {
		u.Logger.Logger.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	logFile, err := homedir.Expand(u.Config.LogFile)
	if err != nil {
		logFile = u.Config.LogFile
	}

	u.Config.LogFile = logFile
	rotate := &rotatorr.Config{
		Filepath: u.Config.LogFile,                                  // log file name.
		FileSize: int64(u.Config.LogFileMb) * megabyte,              // megabytes
		Rotatorr: &timerotator.Layout{FileCount: u.Config.LogFiles}, // number of files to keep.
		DirMode:  logsDirMode,
	}

	switch { // only use MultiWriter if we have > 1 writer.
	case !u.Config.Quiet && u.Config.LogFile != "":
		u.rotatorr = rotatorr.NewMust(rotate)
		u.Logger.Logger.SetOutput(io.MultiWriter(u.rotatorr, os.Stdout))
	case !u.Config.Quiet && u.Config.LogFile == "":
		u.Logger.Logger.SetOutput(os.Stdout)
	case u.Config.LogFile == "":
		u.Logger.Logger.SetOutput(ioutil.Discard) // default is "nothing"
	default:
		u.rotatorr = rotatorr.NewMust(rotate)
		u.Logger.Logger.SetOutput(u.rotatorr)
	}
}

// logStartupInfo prints info about our startup config.
func (u *Unpackerr) logStartupInfo(msg string) {
	u.Printf("==> %s <==", helpLink)
	u.Print("==> Startup Settings <==")
	u.logSonarr()
	u.logRadarr()
	u.logLidarr()
	u.logReadarr()
	u.logFolders()
	u.Printf(" => %s", msg)
	u.Printf(" => Parallel: %d", u.Config.Parallel)
	u.Printf(" => Interval: %v", u.Config.Interval)
	u.Printf(" => Delete Delay: %v", u.Config.DeleteDelay)
	u.Printf(" => Start Delay: %v", u.Config.StartDelay)
	u.Printf(" => Retry Delay: %v, max: %d", u.Config.RetryDelay, u.Config.MaxRetries)
	u.Printf(" => Debug / Quiet: %v / %v", u.Config.Debug, u.Config.Quiet)
	u.Printf(" => Directory & File Modes: %s & %s", u.Config.DirMode, u.Config.FileMode)

	if u.Config.LogFile != "" {
		msg := "no rotation"
		if u.Config.LogFiles > 0 {
			msg = fmt.Sprintf("%d @ %dMb", u.Config.LogFiles, u.Config.LogFileMb)
		}

		u.Printf(" => Log File: %s (%s)", u.Config.LogFile, msg)
	}

	u.logWebhook()
}
