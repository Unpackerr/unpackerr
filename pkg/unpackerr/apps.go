package unpackerr

import (
	"fmt"
	"sync"

	"golift.io/cnfg"
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
	// prefixPathMsg is used to locate/parse a download's path from a text string in StatusMessages.
	prefixPathMsg = "No files found are eligible for import in "
	apiKeyLength  = 32
)

// These are the names used to identify each app.
const (
	Sonarr       = "Sonarr"
	Radarr       = "Radarr"
	Lidarr       = "Lidarr"
	Readarr      = "Readarr"
	FolderString = "Folder"
)

// Application validation errors.
var (
	ErrInvalidURL = fmt.Errorf("provided application URL is invalid")
	ErrInvalidKey = fmt.Errorf("provided application API Key is invalid, must be 32 characters")
)

// Config defines the configuration data used to start the application.
type Config struct {
	Debug       bool             `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Quiet       bool             `json:"quiet" toml:"quiet" xml:"quiet" yaml:"quiet"`
	Parallel    uint             `json:"parallel" toml:"parallel" xml:"parallel" yaml:"parallel"`
	LogFile     string           `json:"logFile" toml:"log_file" xml:"log_file" yaml:"logFile"`
	LogFiles    int              `json:"logFiles" toml:"log_files" xml:"log_files" yaml:"logFiles"`
	LogFileMb   int              `json:"logFileMb" toml:"log_file_mb" xml:"log_file_mb" yaml:"logFileMb"`
	MaxRetries  uint             `json:"maxRetries" toml:"max_retries" xml:"max_retries" yaml:"maxRetries"`
	FileMode    string           `json:"fileMode" toml:"file_mode" xml:"file_mode" yaml:"fileMode"`
	DirMode     string           `json:"dirMode" toml:"dir_mode" xml:"dir_mode" yaml:"dirMode"`
	LogQueues   cnfg.Duration    `json:"logQueues" toml:"log_queues" xml:"log_queues" yaml:"logQueues"` // undocumented.
	Interval    cnfg.Duration    `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout     cnfg.Duration    `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay cnfg.Duration    `json:"deleteDelay" toml:"delete_delay" xml:"delete_delay" yaml:"deleteDelay"`
	StartDelay  cnfg.Duration    `json:"startDelay" toml:"start_delay" xml:"start_delay" yaml:"startDelay"`
	RetryDelay  cnfg.Duration    `json:"retryDelay" toml:"retry_delay" xml:"retry_delay" yaml:"retryDelay"`
	Buffer      uint             `json:"buffer" toml:"buffer" xml:"buffer" yaml:"buffer"`                       //nolint:lll // undocumented.
	KeepHistory uint             `json:"keepHistory" toml:"keep_history" xml:"keep_history" yaml:"keepHistory"` //nolint:lll // undocumented.
	Lidarr      []*LidarrConfig  `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
	Radarr      []*RadarrConfig  `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Readarr     []*ReadarrConfig `json:"readarr,omitempty" toml:"readarr" xml:"readarr" yaml:"readarr,omitempty"`
	Sonarr      []*SonarrConfig  `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Folders     []*FolderConfig  `json:"folder,omitempty" toml:"folder" xml:"folder" yaml:"folder,omitempty"`
	Webhook     []*WebhookConfig `json:"webhook,omitempty" toml:"webhook" xml:"webhook" yaml:"webhook,omitempty"`
	Cmdhook     []*WebhookConfig `json:"cmdhook,omitempty" toml:"cmdhook" xml:"cmdhook" yaml:"cmdhook,omitempty"`
	Folder      struct {
		Interval cnfg.Duration `json:"interval" toml:"interval" xml:"interval" yaml:"interval"` // undocumented.
	} `json:"folders,omitempty" toml:"folders" xml:"folders" yaml:"folders,omitempty"` // undocumented.
}

type workThread struct {
	Funcs []func()
}

func (u *Unpackerr) watchWorkThread() {
	workers := u.Parallel
	if workers > 4 { // nolint:gomnd // 4 == the four starr apps.
		workers = 4
	}

	for i := uint(0); i < workers; i++ {
		go func() {
			for w := range u.workChan {
				for _, f := range w.Funcs {
					f()
				}
			}
		}()
	}
}

// retrieveAppQueues polls Sonarr, Lidarr and Radarr. At the same time.
// Then calls the check methods to scan their queues for changes.
func (u *Unpackerr) retrieveAppQueues() {
	var wg sync.WaitGroup

	// Run each method in a go routine as a waitgroup.
	for _, app := range []func(){u.getLidarrQueue, u.getRadarrQueue, u.getReadarrQueue, u.getSonarrQueue} {
		wg.Add(1)
		u.workChan <- &workThread{[]func(){app, wg.Done}}
	}

	wg.Wait()
	// These are not thread safe because they call handleCompletedDownload.
	u.checkLidarrQueue()
	u.checkRadarrQueue()
	u.checkReadarrQueue()
	u.checkSonarrQueue()
}

// validateApps is broken-out into this file to make adding new apps easier.
func (u *Unpackerr) validateApps() error {
	for _, validate := range []func() error{
		u.validateCmdhook,
		u.validateLidarr,
		u.validateRadarr,
		u.validateReadarr,
		u.validateSonarr,
		u.validateWebhook,
	} {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

func (u *Unpackerr) haveQitem(name, app string) bool {
	switch app {
	case Lidarr:
		return u.haveLidarrQitem(name)
	case Radarr:
		return u.haveRadarrQitem(name)
	case Readarr:
		return u.haveReadarrQitem(name)
	case Sonarr:
		return u.haveSonarrQitem(name)
	default:
		return false
	}
}
