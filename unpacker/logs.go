package unpacker

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

// satisfy gomnd
const (
	callDepth = 2 // log the line that called us.
	oneItem   = 1 // to get pluralization right.
)

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

	u.Logf("[Unpackerr] Queue: [%d waiting] [%d queued] [%d extracting] [%d extracted] [%d imported]"+
		" [%d failed] [%d deleted], Totals: [%d restarts] [%d finished]",
		e.waiting, e.queued, e.extracting, e.extracted, e.imported, e.failed, e.deleted,
		u.Restarted, u.Finished)
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
			return err
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

	if c := len(u.Sonarr); c == oneItem {
		u.Logf(" => Sonarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Sonarr[0].URL, u.Sonarr[0].Path, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout)
	} else {
		u.Log(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Radarr); c == oneItem {
		u.Logf(" => Radarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v)",
			u.Radarr[0].URL, u.Radarr[0].Path, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout)
	} else {
		u.Log(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v)",
				f.URL, f.Path, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Lidarr); c == oneItem {
		u.Logf(" => Lidarr Config: 1 server: %s (apikey: %v, timeout: %v)",
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout)
	} else {
		u.Log(" => Lidarr Config:", c, "servers")

		for _, f := range u.Lidarr {
			u.Logf(" =>    Server: %s (apikey: %v, timeout: %v)", f.URL, f.APIKey != "", f.Timeout)
		}
	}

	if c := len(u.Folders); c == oneItem {
		u.Logf(" => Folder Config: 1 path: %s (delete after:%v, delete orig:%v, move back:%v)",
			u.Folders[0].Path, u.Folders[0].DeleteAfter, u.Folders[0].DeleteOrig, u.Folders[0].MoveBack)
	} else {
		u.Log(" => Folder Config:", c, "paths")

		for _, f := range u.Folders {
			u.Logf(" =>    Path: %s (delete after:%v, delete orig:%v, move back:%v)",
				f.Path, f.DeleteAfter, f.DeleteOrig, f.MoveBack)
		}
	}

	u.Log(" => Parallel:", u.Config.Parallel)
	u.Log(" => Interval:", u.Config.Interval.Duration)
	u.Log(" => Delete Delay:", u.Config.DeleteDelay.Duration)
	u.Log(" => Start Delay:", u.Config.StartDelay.Duration)
	u.Log(" => Retry Delay:", u.Config.RetryDelay.Duration)
	u.Log(" => Debug / Quiet:", u.Config.Debug, "/", u.Config.Quiet)
	u.Log(" => Log File:", u.Config.LogFile)
}