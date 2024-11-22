package unpackerr

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
)

/* This file contains all the unique bits for each app. When adding a new app,
   duplicate the lidarr.go file and rename all the things, then add the new app
   to the various places below, in this file.
*/

// DefaultQueuePageSize is how many items we request from Lidarr and Readarr.
// Once we have better support for Sonarr/Radarr v3 this will apply to those as well.
// If you have more than this many items queued.. oof.
// As the queue goes away, more things should get picked up.
const DefaultQueuePageSize = 2000

const (
	defaultProtocol = "torrent"
	apiKeyLength    = 32
)

// These are the names used to identify each app.
const (
	FolderString = "Folder"
)

// Application validation errors.
var (
	ErrInvalidURL = errors.New("provided application URL is invalid")
	ErrInvalidKey = fmt.Errorf("provided application API Key is invalid, must be %d characters", apiKeyLength)
)

// Config defines the configuration data used to start the application.
//
//nolint:lll
type Config struct {
	Debug       bool             `json:"debug"              toml:"debug"        xml:"debug"        yaml:"debug"`
	Quiet       bool             `json:"quiet"              toml:"quiet"        xml:"quiet"        yaml:"quiet"`
	Activity    bool             `json:"activity"           toml:"activity"     xml:"activity"     yaml:"activity"`
	Parallel    uint             `json:"parallel"           toml:"parallel"     xml:"parallel"     yaml:"parallel"`
	ErrorStdErr bool             `json:"errorStderr"        toml:"error_stderr" xml:"error_stderr" yaml:"errorStderr"`
	LogFile     string           `json:"logFile"            toml:"log_file"     xml:"log_file"     yaml:"logFile"`
	LogFiles    int              `json:"logFiles"           toml:"log_files"    xml:"log_files"    yaml:"logFiles"`
	LogFileMb   int              `json:"logFileMb"          toml:"log_file_mb"  xml:"log_file_mb"  yaml:"logFileMb"`
	MaxRetries  uint             `json:"maxRetries"         toml:"max_retries"  xml:"max_retries"  yaml:"maxRetries"`
	FileMode    string           `json:"fileMode"           toml:"file_mode"    xml:"file_mode"    yaml:"fileMode"`
	DirMode     string           `json:"dirMode"            toml:"dir_mode"     xml:"dir_mode"     yaml:"dirMode"`
	LogQueues   cnfg.Duration    `json:"logQueues"          toml:"log_queues"   xml:"log_queues"   yaml:"logQueues"`
	Interval    cnfg.Duration    `json:"interval"           toml:"interval"     xml:"interval"     yaml:"interval"`
	Timeout     cnfg.Duration    `json:"timeout"            toml:"timeout"      xml:"timeout"      yaml:"timeout"`
	DeleteDelay cnfg.Duration    `json:"deleteDelay"        toml:"delete_delay" xml:"delete_delay" yaml:"deleteDelay"`
	StartDelay  cnfg.Duration    `json:"startDelay"         toml:"start_delay"  xml:"start_delay"  yaml:"startDelay"`
	RetryDelay  cnfg.Duration    `json:"retryDelay"         toml:"retry_delay"  xml:"retry_delay"  yaml:"retryDelay"`
	KeepHistory uint             `json:"keepHistory"        toml:"keep_history" xml:"keep_history" yaml:"keepHistory"` // undocumented.
	Passwords   StringSlice      `json:"passwords"          toml:"passwords"    xml:"password"     yaml:"passwords"`
	Webserver   *WebServer       `json:"webserver"          toml:"webserver"    xml:"webserver"    yaml:"webserver"`
	Lidarr      []*LidarrConfig  `json:"lidarr,omitempty"   toml:"lidarr"       xml:"lidarr"       yaml:"lidarr,omitempty"`
	Radarr      []*RadarrConfig  `json:"radarr,omitempty"   toml:"radarr"       xml:"radarr"       yaml:"radarr,omitempty"`
	Whisparr    []*RadarrConfig  `json:"whisparr,omitempty" toml:"whisparr"     xml:"whisparr"     yaml:"whisparr,omitempty"`
	Readarr     []*ReadarrConfig `json:"readarr,omitempty"  toml:"readarr"      xml:"readarr"      yaml:"readarr,omitempty"`
	Sonarr      []*SonarrConfig  `json:"sonarr,omitempty"   toml:"sonarr"       xml:"sonarr"       yaml:"sonarr,omitempty"`
	Folders     []*FolderConfig  `json:"folder,omitempty"   toml:"folder"       xml:"folder"       yaml:"folder,omitempty"`
	Webhook     []*WebhookConfig `json:"webhook,omitempty"  toml:"webhook"      xml:"webhook"      yaml:"webhook,omitempty"`
	Cmdhook     []*WebhookConfig `json:"cmdhook,omitempty"  toml:"cmdhook"      xml:"cmdhook"      yaml:"cmdhook,omitempty"`
	Folder      FoldersConfig    `json:"folders,omitempty"  toml:"folders"      xml:"folders"      yaml:"folders,omitempty"` // undocumented.
}

type FoldersConfig struct {
	Buffer   uint          `json:"buffer"   toml:"buffer"   xml:"buffer"   yaml:"buffer"`   // undocumented.
	Interval cnfg.Duration `json:"interval" toml:"interval" xml:"interval" yaml:"interval"` // undocumented.
}

func (u *Unpackerr) watchWorkThread() {
	// 1 worker for each app, so they poll quickly.
	for range len(u.Lidarr) + len(u.Radarr) + len(u.Readarr) + len(u.Sonarr) + len(u.Whisparr) {
		go func() {
			for funcs := range u.workChan {
				for _, fn := range funcs {
					fn()
				}
			}
		}()
	}
}

// retrieveAppQueues polls all the starr app queues. At the same time.
// Then calls the check methods to scan their queue contents for changes.
func (u *Unpackerr) retrieveAppQueues(now time.Time) {
	wait := sync.WaitGroup{}
	wait.Add(len(u.Lidarr) + len(u.Radarr) + len(u.Readarr) + len(u.Sonarr) + len(u.Whisparr))
	// Run each app's getQueue method in a go routine as a waitgroup.
	for _, server := range u.Lidarr {
		u.workChan <- []func(){func() { u.getLidarrQueue(server, now) }, wait.Done}
	}

	for _, server := range u.Radarr {
		u.workChan <- []func(){func() { u.getRadarrQueue(server, now) }, wait.Done}
	}

	for _, server := range u.Readarr {
		u.workChan <- []func(){func() { u.getReadarrQueue(server, now) }, wait.Done}
	}

	for _, server := range u.Sonarr {
		u.workChan <- []func(){func() { u.getSonarrQueue(server, now) }, wait.Done}
	}

	for _, server := range u.Whisparr {
		u.workChan <- []func(){func() { u.getWhisparrQueue(server, now) }, wait.Done}
	}

	wait.Wait()
	// These are not thread safe because they call saveCompletedDownload.
	u.checkLidarrQueue(now)
	u.checkRadarrQueue(now)
	u.checkReadarrQueue(now)
	u.checkSonarrQueue(now)
	u.checkWhisparrQueue(now)
}

// validateApps is broken-out into this file to make adding new apps easier.
func (u *Unpackerr) validateApps() error {
	for _, validate := range []func() error{
		u.validateLidarr,
		u.validateRadarr,
		u.validateReadarr,
		u.validateSonarr,
		u.validateWhisparr,
	} {
		if err := validate(); err != nil {
			return err
		}
	}

	for _, validate := range []func() error{
		u.validateCmdhook,
		u.validateWebhook,
	} {
		if err := validate(); err != nil {
			u.Errorf("Config Warning: %v", err)
		}
	}

	return nil
}

func (u *Unpackerr) haveQitem(name string, app starr.App) bool {
	switch app {
	case starr.Lidarr:
		return u.haveLidarrQitem(name)
	case starr.Radarr:
		return u.haveRadarrQitem(name)
	case starr.Readarr:
		return u.haveReadarrQitem(name)
	case starr.Sonarr:
		return u.haveSonarrQitem(name)
	case starr.Whisparr:
		return u.haveWhisparrQitem(name)
	default:
		return false
	}
}

// StringSlice allows a special environment variable unmarshaller for a lot of strings.
type StringSlice []string

// UnmarshalENV turns environment variables into a string slice.
func (slice *StringSlice) UnmarshalENV(_, envval string) error {
	if envval == "" {
		return nil
	}

	envval = strings.Trim(envval, `["',] `)
	vals := strings.Split(envval, ",")
	*slice = make(StringSlice, len(vals))

	for idx, val := range vals {
		(*slice)[idx] = strings.TrimSpace(val)
	}

	return nil
}

func (slice *StringSlice) MarshalENV(tag string) (map[string]string, error) {
	return map[string]string{tag: strings.Join(*slice, ",")}, nil
}

func buildStatusReason(status string, messages []*starr.StatusMessage) string {
	var output string

	for idx := range messages {
		for _, msg := range messages[idx].Messages {
			if output != "" {
				output += "; "
			}

			output += msg
		}
	}

	return status + ": " + output
}
