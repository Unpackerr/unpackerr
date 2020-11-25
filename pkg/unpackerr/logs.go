package unpackerr

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
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

// Debug writes Debug log lines... to stdout and/or a file.
func (l *Logger) Debugf(msg string, v ...interface{}) {
	if l.debug {
		_ = l.Logger.Output(callDepth, "[DEBUG] "+fmt.Sprintf(msg, v...))
	}
}

// Log writes log lines... to stdout and/or a file.
func (l *Logger) Log(v ...interface{}) {
	_ = l.Logger.Output(callDepth, fmt.Sprintln(v...))
}

// Logf writes log lines... to stdout and/or a file.
func (l *Logger) Printf(msg string, v ...interface{}) {
	_ = l.Logger.Output(callDepth, fmt.Sprintf(msg, v...))
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
	u.Printf("[Unpackerr] Totals: [%d restarted] [%d finished] [%d|%d webhooks] [%d stacks]",
		u.Restarted, u.Finished, hookOK, hookFail, len(u.folders.Events)+len(u.updates)+len(u.folders.Updates))
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() {
	u.Logger.debug = u.Config.Debug

	if u.Logger.Logger.SetFlags(log.LstdFlags); u.Config.Debug {
		u.Logger.Logger.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	switch { // only use MultiWriter is we have > 1 writer.
	case !u.Config.Quiet && u.Config.LogFile != "":
		u.Logger.Logger.SetOutput(io.MultiWriter(&lumberjack.Logger{
			Filename:   u.Config.LogFile,   // log file name.
			MaxSize:    u.Config.LogFileMb, // megabytes
			MaxBackups: u.Config.LogFiles,  // number of files to keep.
			MaxAge:     0,                  // days, 0 for unlimited
			Compress:   false,              // meh no thanks.
			LocalTime:  true,               // use local time in logs, not UTC.
		}, os.Stdout))
	case !u.Config.Quiet && u.Config.LogFile == "":
		u.Logger.Logger.SetOutput(os.Stdout)
	case u.Config.LogFile == "":
		u.Logger.Logger.SetOutput(ioutil.Discard) // default is "nothing"
	default:
		u.Logger.Logger.SetOutput(&lumberjack.Logger{
			Filename:   u.Config.LogFile,
			MaxSize:    u.Config.LogFileMb, // megabytes
			MaxBackups: u.Config.LogFiles,
			MaxAge:     0,     // days, 0 for unlimited
			Compress:   false, // meh no thanks.
			LocalTime:  true,  // use local time in logs, not UTC.
		})
	}
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
