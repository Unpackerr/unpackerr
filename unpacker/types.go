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
	Debug       bool          `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Interval    cnfg.Duration `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout     cnfg.Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay cnfg.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Parallel    uint          `json:"parallel" toml:"parallel" xml:"parallel" yaml:"parallel"`
	SonarrPath  string        `json:"sonar_path" toml:"sonar_path" xml:"sonar_path" yaml:"sonar_path"`
	RadarrPath  string        `json:"radar_path" toml:"radar_path" xml:"radar_path" yaml:"radar_path"`
	Sonarr      *starr.Config `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Radarr      *starr.Config `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Lidarr      *starr.Config `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
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
	finished   uint
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
	*SonarrQ
	*RadarrQ
	*History
	SigChan  chan os.Signal
	StopChan chan bool
}

// SonarrQ holds the queued items in the Sonarr activity list.
type SonarrQ struct {
	sync.RWMutex
	List []*starr.SonarQueue
}

// RadarrQ holds the queued items in the Radarr activity list.
type RadarrQ struct {
	sync.RWMutex
	List []*starr.RadarQueue
}

// History holds the history of extracted items.
type History struct {
	sync.RWMutex
	Finished uint
	Map      map[string]Extracts
}

// Extracts holds data for files being extracted.
type Extracts struct {
	Path    string
	App     string
	Files   []string
	Status  ExtractStatus
	Updated time.Time
}
