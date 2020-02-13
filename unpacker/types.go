package unpacker

import (
	"os"
	"sync"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Debug       bool            `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Parallel    uint            `json:"parallel" toml:"parallel" xml:"parallel" yaml:"parallel"`
	Interval    cnfg.Duration   `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout     cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay cnfg.Duration   `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	StartDelay  cnfg.Duration   `json:"start_delay" toml:"start_delay" xml:"start_delay" yaml:"start_delay"`
	RetryDelay  cnfg.Duration   `json:"retry_delay" toml:"retry_delay" xml:"retry_delay" yaml:"retry_delay"`
	Sonarr      []*sonarrConfig `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Radarr      []*radarrConfig `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Lidarr      []*lidarrConfig `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
}

type radarrConfig struct {
	*starr.Config
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
	List         []*starr.RadarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type sonarrConfig struct {
	*starr.Config
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
	List         []*starr.SonarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type lidarrConfig struct {
	*starr.Config
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
	List         []*starr.LidarrRecord `json:"-" toml:"-" xml:"-" yaml:"-"`
}

// ExtractStatus is our enum for an extract's status.
type ExtractStatus uint8

// Extract Statuses.
const (
	MISSING = ExtractStatus(iota)
	QUEUED
	EXTRACTING
	EXTRACTFAILED
	EXTRACTFAILED2
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
		"Missing", "Queued", "Extraction Progressing", "Extraction Failed",
		"Extraction Failed Twice", "Extracted, Awaiting Import", "Imported",
		"Deleting", "Delete Failed", "Deleted",
	}[status]
}

// Use in r.eCount to return activity counters.
type eCounters struct {
	queued     uint
	extracting uint
	failed     uint
	extracted  uint
	imported   uint
	deleted    uint
}

// Flags are our CLI input flags.
type Flags struct {
	verReq     bool
	ConfigFile string
}

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	SigChan chan os.Signal
}

// History holds the history of extracted items.
type History struct {
	sync.RWMutex
	Finished  uint
	Restarted uint
	Map       map[string]Extracts
}

// Extracts holds data for files being extracted.
type Extracts struct {
	Path    string
	App     string
	Files   []string
	Status  ExtractStatus
	Updated time.Time
}
