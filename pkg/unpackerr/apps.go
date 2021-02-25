package unpackerr

import (
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
)

// These are the names used to identify each app.
const (
	Sonarr       = "Sonarr"
	Radarr       = "Radarr"
	Lidarr       = "Lidarr"
	Readarr      = "Readarr"
	FolderString = "Folder"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Debug       bool             `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Quiet       bool             `json:"quiet" toml:"quiet" xml:"quiet" yaml:"quiet"`
	Parallel    uint             `json:"parallel" toml:"parallel" xml:"parallel" yaml:"parallel"`
	LogFile     string           `json:"log_file" toml:"log_file" xml:"log_file" yaml:"log_file"`
	LogFiles    int              `json:"log_files" toml:"log_files" xml:"log_files" yaml:"log_files"`
	LogFileMb   int              `json:"log_file_mb" toml:"log_file_mb" xml:"log_file_mb" yaml:"log_file_mb"`
	MaxRetries  uint             `json:"max_retries" toml:"max_retries" xml:"max_retries" yaml:"max_retries"`
	FileMode    string           `json:"file_mode" toml:"file_mode" xml:"file_mode" yaml:"file_mode"`
	DirMode     string           `json:"dir_mode" toml:"dir_mode" xml:"dir_mode" yaml:"dir_mode"`
	LogQueues   cnfg.Duration    `json:"log_queues" toml:"log_queues" xml:"log_queues" yaml:"log_queues"` // undocumented.
	Interval    cnfg.Duration    `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout     cnfg.Duration    `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay cnfg.Duration    `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	StartDelay  cnfg.Duration    `json:"start_delay" toml:"start_delay" xml:"start_delay" yaml:"start_delay"`
	RetryDelay  cnfg.Duration    `json:"retry_delay" toml:"retry_delay" xml:"retry_delay" yaml:"retry_delay"`
	Buffer      uint             `json:"buffer" toml:"buffer" xml:"buffer" yaml:"buffer"` // undocumented.
	Sonarr      []*SonarrConfig  `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Radarr      []*RadarrConfig  `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Lidarr      []*LidarrConfig  `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
	Readarr     []*ReadarrConfig `json:"readarr,omitempty" toml:"readarr" xml:"readarr" yaml:"readarr,omitempty"`
	Folders     []*FolderConfig  `json:"folder,omitempty" toml:"folder" xml:"folder" yaml:"folder,omitempty"`
	Webhook     []*WebhookConfig `json:"webhook,omitempty" toml:"webhook" xml:"webhook" yaml:"webhook,omitempty"`
}

// retrieveAppQueues polls Sonarr, Lidarr and Radarr. At the same time.
// The calls the check methods to scan their queues for changes.
func (u *Unpackerr) retrieveAppQueues() {
	var wg sync.WaitGroup

	// Run each method in a go routine as a waitgroup.
	for _, f := range []func(){
		u.getSonarrQueue,
		u.getRadarrQueue,
		u.getLidarrQueue,
		u.getReadarrQueue,
	} {
		wg.Add(1)

		go func(f func()) {
			f()
			wg.Done()
		}(f)
	}

	wg.Wait()
	// These are not thread safe because they call handleCompletedDownload.
	u.checkSonarrQueue()
	u.checkRadarrQueue()
	u.checkLidarrQueue()
	u.checkReadarrQueue()
}

// validateApps is broken-out into this file to make adding new apps easier.
func (u *Unpackerr) validateApps() error {
	u.validateSonarr()
	u.validateRadarr()
	u.validateLidarr()
	u.validateReadarr()

	return u.validateWebhook()
}

func (u *Unpackerr) haveQitem(name, app string) bool {
	switch app {
	case Sonarr:
		return u.haveSonarrQitem(name)
	case Radarr:
		return u.haveRadarrQitem(name)
	case Lidarr:
		return u.haveLidarrQitem(name)
	case Readarr:
		return u.haveReadarrQitem(name)
	default:
		return false
	}
}
