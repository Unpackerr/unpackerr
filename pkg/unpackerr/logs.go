package unpackerr

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/dromara/carbon/v2"
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

// UnmarshalText turns a json identifier or TOML event ID back into a status.
func (status *ExtractStatus) UnmarshalText(text []byte) error {
	name := strings.TrimSpace(string(text))
	if parsed, err := strconv.ParseUint(name, 10, 8); err == nil {
		got := ExtractStatus(parsed)
		if got <= EXTRACTEDNOTHING {
			*status = got
			return nil
		}
	}

	for candidate := WAITING; candidate <= EXTRACTEDNOTHING; candidate++ {
		if candidate.String() == name {
			*status = candidate
			return nil
		}
	}

	return fmt.Errorf("%w: %s", errUnknownExtractStatus, name)
}

var errUnknownExtractStatus = errors.New("unknown extract status")

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
		stats.Retries, stats.Finished, stats.HookOK, stats.HookFail, stats.CmdOK, stats.CmdFail,
		len(u.folders.Events)+len(u.updates)+len(u.folders.Updates), len(u.hookChan), len(u.delChan),
		carbon.CreateFromStdTime(version.Started).DiffAbsInString(carbon.CreateFromStdTime(now)))
	u.updateTray(stats, uint(len(u.folders.Events)+len(u.updates)+len(u.folders.Updates)+len(u.delChan)+len(u.hookChan)))
}

// setupLogging splits log write into a file and/or stdout.
func (u *Unpackerr) setupLogging() {
	if u.Config.Debug {
		u.Info.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
		u.Error.SetFlags(log.Lshortfile | log.Lmicroseconds | log.Ldate)
	}

	u.LogFile = getLogFilePath(u.LogFile, "unpackerr.log")
	fileMode, _ := strconv.ParseUint(u.LogFileMode, bits8, base32)
	rotate := &rotatorr.Config{
		Filepath: u.LogFile,
		FileSize: logFileSize(u.LogFiles, u.LogFileMb),
		Rotatorr: &timerotator.Layout{
			FileCount:  u.LogFiles,
			PostRotate: u.postLogRotate,
		},
		DirMode:  logsDirMode,
		FileMode: os.FileMode(fileMode),
	}

	if u.LogFile != "" {
		var err error

		u.rotatorr, err = rotatorr.New(rotate)
		if err != nil {
			// Fall back to stdout so we don't hammer the filesystem with failed open attempts.
			u.rotatorr = nil
			_, _ = os.Stdout.WriteString("[Unpackerr] Log file unavailable (check path and permissions!!), " +
				"logging to stdout only: " + err.Error() + "\n")
		}
	}

	stderr := os.Stdout
	if u.ErrorStdErr {
		stderr = os.Stderr
	}

	useLogFile := u.LogFile != "" && u.rotatorr != nil

	switch { // only use MultiWriter if we have > 1 writer.
	case !u.Quiet && useLogFile:
		u.updateLogOutput(io.MultiWriter(u.rotatorr, os.Stdout), io.MultiWriter(u.rotatorr, stderr))
	case !u.Quiet && !useLogFile:
		u.updateLogOutput(os.Stdout, stderr)
	case !useLogFile:
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

func logFileSize(files, megabytes int) int64 {
	if files <= 0 {
		return rotatorr.NoMaxSize
	}

	return int64(megabytes) * megabyte
}

func (u *Unpackerr) waitForExit() {
	for {
		sig := <-u.sigChan
		if isHangup(sig) {
			u.reopenLogs()

			continue
		}

		u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, sig)

		return
	}
}

func (u *Unpackerr) reopenLogs() {
	reopened := true

	if u.rotatorr != nil {
		if err := u.rotatorr.Reopen(); err != nil {
			u.Errorf("Reopening log file: %v", err)

			reopened = false
		}
	}

	if u.httpLog != nil {
		if err := u.httpLog.Reopen(); err != nil {
			u.Errorf("Reopening HTTP log file: %v", err)

			reopened = false
		}
	}

	if !reopened {
		return
	}

	// After Reopen so this lands in the live file, not the one logrotate just moved.
	u.Printf("Caught SIGHUP: reopened log files")
}

func (u *Unpackerr) updateLogOutput(writer io.Writer, errors io.Writer) {
	if u.Webserver != nil && u.Webserver.LogFile != "" {
		u.setupHTTPLogging()
	} else {
		u.HTTP.SetOutput(writer)
	}

	if u.Config.Debug {
		u.Logger.Debug.SetOutput(writer)
	}

	log.SetOutput(errors) // catch out-of-scope garbage
	u.Info.SetOutput(writer)
	u.Error.SetOutput(errors)
	u.postLogRotate("", "")
}

func (u *Unpackerr) setupHTTPLogging() {
	u.Webserver.LogFile = getLogFilePath(u.Webserver.LogFile, "http.log")
	rotate := &rotatorr.Config{
		Filepath: u.Webserver.LogFile,
		FileSize: logFileSize(u.Webserver.LogFiles, u.Webserver.LogFileMb),
		Rotatorr: &timerotator.Layout{FileCount: u.Webserver.LogFiles},
		DirMode:  logsDirMode,
	}

	u.httpLog = rotatorr.NewMust(rotate)

	switch { // only use MultiWriter if we have > 1 writer.
	case !u.Quiet && u.Webserver.LogFile != "":
		u.HTTP.SetOutput(io.MultiWriter(u.httpLog, os.Stdout))
	case !u.Quiet && u.Webserver.LogFile == "":
		u.HTTP.SetOutput(os.Stdout)
	case u.Quiet && u.Webserver.LogFile == "":
		u.HTTP.SetOutput(io.Discard)
	default: // u.Config.Quiet && u.Webserver.LogFile != ""
		u.HTTP.SetOutput(u.httpLog)
	}
}

func (u *Unpackerr) postLogRotate(_, newFile string) {
	if newFile != "" {
		// Post runs on rotatorr's dispatch goroutine; a sync Printf deadlocks.
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
	u.Printf(" => Parallel: %d", u.Parallel)

	u.Printf(" => Default Extract Limits: Sonarr/Whisparr %s, Radarr %s, Lidarr %s, Readarr %s; "+
		"%d files, %g:1, %d nested, extras depth %d; folders uncapped",
		defaultSonarrMaxBytes, defaultRadarrMaxBytes, defaultLidarrMaxBytes, defaultReadarrMaxBytes,
		defaultMaxFiles, defaultMaxRatio, defaultMaxNested, defaultExtrasMaxDepth)

	u.Printf(" => Passwords: %d (rar/7z)", len(u.Passwords))
	u.Printf(" => Interval / Progress: %s/%s", u.Interval.String(), u.Progress.String())
	u.Printf(" => Start/Delete Delay: %s/%s", u.StartDelay.String(), u.DeleteDelay.String())
	u.Printf(" => Retry Delay: %v, max: %d", u.RetryDelay, u.maxRetries())
	u.Printf(" => Remnant Action: %s", u.RemnantAction)
	u.Printf(" => GUI / StdErr: %v / %v", ui.HasGUI(), u.ErrorStdErr)
	u.Printf(" => Debug / Quiet: %v / %v", u.Config.Debug, u.Quiet)
	u.Printf(" => Activity / Queues: %v / %s", u.Activity, u.LogQueues.String())

	if runtime.GOOS != windows {
		u.Printf(" => Directory & File Modes: %s & %s", u.DirMode, u.FileMode)
	}

	if u.LogFile != "" {
		msg := "no rotation"
		if u.LogFiles > 0 {
			msg = fmt.Sprintf("%d @ %dMb", u.LogFiles, u.LogFileMb)
		}

		u.Printf(" => Log File: %s (%s, mode: %s)", u.LogFile, msg, u.LogFileMode)
	}

	u.logWebhook()
	u.logCmdhook()
	u.logWebserver()
}
