package unpackerr

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/hako/durafmt"
	homedir "github.com/mitchellh/go-homedir"
	"golift.io/rotatorr"
	"golift.io/rotatorr/timerotator"
	"golift.io/version"
)

// satisfy gomnd.
const (
	callDepth    = 2 // log the line that called us.
	megabyte     = 1024 * 1024
	logsDirMode  = 0o755
	starrLogPfx  = " =>    Server: "
	starrLogLine = "%s, apikey:%v, timeout:%v, verify_ssl:%v, protos:%s, " +
		"syncthing:%v, delete_orig:%v, delete_delay:%v, paths:%q"
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
	EXTRACTEDNOTHING
)

// Desc makes ExtractStatus human readable.
func (status ExtractStatus) Desc() string {
	if status > EXTRACTEDNOTHING {
		return "Unknown"
	}

	return []string{
		// The order must not be faulty.
		"Waiting, pre-Queue",
		"Queued",
		"Extracting",
		"Extraction Failed",
		"Extracted, Awaiting Import",
		"Imported",
		"Deleting",
		"Delete Failed",
		"Deleted",
		"Nothing Extracted",
	}[status]
}

// MarshalText turns a status into a word, for a json identifier.
func (status ExtractStatus) MarshalText() ([]byte, error) {
	return []byte(status.String()), nil
}

// String turns a status into a short string.
func (status ExtractStatus) String() string {
	if status > EXTRACTEDNOTHING {
		return "unknown"
	}

	return []string{
		// The order must not be faulty.
		"waiting",
		"queued",
		"extracting",
		"extractfailed",
		"extracted",
		"imported",
		"deleting",
		"deletefailed",
		"deleted",
		"extractednothing",
	}[status]
}

// Debugf writes Debug log lines... to stdout and/or a file.
func (l *Logger) Debugf(msg string, v ...any) {
	err := l.Debug.Output(callDepth, fmt.Sprintf(msg, v...))
	if err != nil {
		fmt.Println("Logger Error:", err) //nolint:forbidigo
	}
}

// Printf writes log lines... to stdout and/or a file.
func (l *Logger) Printf(msg string, v ...any) {
	err := l.Info.Output(callDepth, fmt.Sprintf(msg, v...))
	if err != nil {
		fmt.Println("Logger Error:", err) //nolint:forbidigo
	}
}

// Errorf writes log errors... to stdout and/or a file.
func (l *Logger) Errorf(msg string, v ...any) {
	err := l.Error.Output(callDepth, fmt.Sprintf(msg, v...))
	if err != nil {
		fmt.Println("Logger Error:", err) //nolint:forbidigo
	}
}

// logCurrentQueue prints the number of things happening.
func (u *Unpackerr) logCurrentQueue(now time.Time) {
	stats := u.stats()
	u.Printf("[Unpackerr] Queue: %d waiting, %d queued, %d extracting, %d extracted, %d imported, %d failed, %d deleted",
		stats.Waiting, stats.Queued, stats.Extracting, stats.Extracted, stats.Imported, stats.Failed, stats.Deleted)

	u.Printf("[Unpackerr] Totals: %d retries, %d finished, %d|%d webhooks,"+
		" %d|%d cmdhooks, stacks; event:%d, hook:%d, del:%d, up %s",
		u.Retries, u.Finished, stats.HookOK, stats.HookFail, stats.CmdOK, stats.CmdFail,
		len(u.folders.Events)+len(u.updates)+len(u.folders.Updates), len(u.hookChan), len(u.delChan),
		durafmt.Parse(now.Sub(version.Started)).LimitFirstN(3).Format(durafmtUnits)) //nolint:mnd

	//nolint:gosec
	u.updateTray(stats, uint(len(u.folders.Events)+len(u.updates)+len(u.folders.Updates)+len(u.delChan)+len(u.hookChan)))
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() {
	if u.Config.Debug {
		u.Logger.Info.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
		u.Logger.Error.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	u.Config.LogFile = getLogFilePath(u.Config.LogFile, "unpackerr.log")
	fileMode, _ := strconv.ParseUint(u.LogFileMode, bits8, base32)
	rotate := &rotatorr.Config{
		Filepath: u.Config.LogFile,                     // log file name.
		FileSize: int64(u.Config.LogFileMb) * megabyte, // megabytes
		Rotatorr: &timerotator.Layout{
			FileCount:  u.Config.LogFiles,
			PostRotate: u.postLogRotate,
		}, // number of files to keep.
		DirMode:  logsDirMode,
		FileMode: os.FileMode(fileMode),
	}

	if u.Config.LogFile != "" {
		u.rotatorr = rotatorr.NewMust(rotate)
	}

	stderr := os.Stdout
	if u.ErrorStdErr {
		stderr = os.Stderr
	}

	switch { // only use MultiWriter if we have > 1 writer.
	case !u.Config.Quiet && u.Config.LogFile != "":
		u.updateLogOutput(io.MultiWriter(u.rotatorr, os.Stdout), io.MultiWriter(u.rotatorr, stderr))
	case !u.Config.Quiet && u.Config.LogFile == "":
		u.updateLogOutput(os.Stdout, stderr)
	case u.Config.LogFile == "":
		u.updateLogOutput(io.Discard, io.Discard) // default is "nothing"
	default:
		u.updateLogOutput(u.rotatorr, u.rotatorr)
	}
}

// getLogFilePath takes in a path and a base name. In case the path is a directory, they are joined.
func getLogFilePath(logFile, base string) string {
	if expanded, err := homedir.Expand(logFile); err == nil {
		logFile = expanded
	}

	if stat, err := os.Stat(logFile); err == nil && stat.IsDir() {
		return filepath.Join(logFile, base)
	}

	return logFile
}

func (u *Unpackerr) updateLogOutput(writer io.Writer, errors io.Writer) {
	if u.Webserver != nil && u.Webserver.LogFile != "" {
		u.setupHTTPLogging()
	} else {
		u.Logger.HTTP.SetOutput(writer)
	}

	if u.Config.Debug {
		u.Logger.Debug.SetOutput(writer)
	}

	log.SetOutput(errors) // catch out-of-scope garbage
	u.Logger.Info.SetOutput(writer)
	u.Logger.Error.SetOutput(errors)
	u.postLogRotate("", "")
}

func (u *Unpackerr) setupHTTPLogging() {
	u.Webserver.LogFile = getLogFilePath(u.Webserver.LogFile, "http.log")
	rotate := &rotatorr.Config{
		Filepath: u.Webserver.LogFile,                     // log file name.
		FileSize: int64(u.Webserver.LogFileMb) * megabyte, // megabytes
		Rotatorr: &timerotator.Layout{FileCount: u.Webserver.LogFiles},
		DirMode:  logsDirMode,
	}

	switch { // only use MultiWriter if we have > 1 writer.
	case !u.Config.Quiet && u.Webserver.LogFile != "":
		u.Logger.HTTP.SetOutput(io.MultiWriter(rotatorr.NewMust(rotate), os.Stdout))
	case !u.Config.Quiet && u.Webserver.LogFile == "":
		u.Logger.HTTP.SetOutput(os.Stdout)
	case u.Config.Quiet && u.Webserver.LogFile == "":
		u.Logger.HTTP.SetOutput(io.Discard)
	default: // u.Config.Quiet && u.Webserver.LogFile != ""
		u.Logger.HTTP.SetOutput(rotatorr.NewMust(rotate))
	}
}

func (u *Unpackerr) postLogRotate(_, newFile string) {
	if newFile != "" {
		go u.Printf("Rotated log file to: %s", newFile)
	}

	if u.rotatorr != nil && u.rotatorr.File != nil {
		redirectStderr(u.rotatorr.File) // Log panics.
	}
}

// logStartupInfo prints info about our startup config.
func (u *Unpackerr) logStartupInfo(msg string, externalFiles map[string]string) {
	u.Printf("==> %s <==", helpLink)
	u.Printf("==> Startup Settings <==")
	u.Printf(" => %s", msg)

	for path, file := range externalFiles {
		u.Printf(" => Extra Config File: %s => %s", file, path)
	}

	u.logSonarr()
	u.logRadarr()
	u.logLidarr()
	u.logReadarr()
	u.logWhisparr()
	u.logFolders()
	u.Printf(" => Parallel: %d", u.Config.Parallel)
	u.Printf(" => Passwords: %d (rar/7z)", len(u.Config.Passwords))
	u.Printf(" => Interval: %v", u.Config.Interval)
	u.Printf(" => Start/Delete Delay: %v/%v", u.Config.StartDelay, u.Config.DeleteDelay)
	u.Printf(" => Retry Delay: %v, max: %d", u.Config.RetryDelay, u.Config.MaxRetries)
	u.Printf(" => GUI / StdErr: %v / %v", ui.HasGUI(), u.ErrorStdErr)
	u.Printf(" => Debug / Quiet: %v / %v", u.Config.Debug, u.Config.Quiet)
	u.Printf(" => Activity / Queues: %v / %v", u.Config.Activity, u.Config.LogQueues)

	if runtime.GOOS != windows {
		u.Printf(" => Directory & File Modes: %s & %s", u.Config.DirMode, u.Config.FileMode)
	}

	if u.Config.LogFile != "" {
		msg := "no rotation"
		if u.Config.LogFiles > 0 {
			msg = fmt.Sprintf("%d @ %dMb", u.Config.LogFiles, u.Config.LogFileMb)
		}

		u.Printf(" => Log File: %s (%s, mode: %s)", u.LogFile, msg, u.LogFileMode)
	}

	u.logWebhook()
	u.logCmdhook()
	u.logWebserver()
}
