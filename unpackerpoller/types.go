package unpackerpoller

import (
	"os"
	"sync"
	"time"

	"golift.io/deluge"
	"golift.io/starr"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Debug              bool           `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Interval           starr.Duration `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout            starr.Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay        starr.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	ConcurrentExtracts uint           `json:"concurrent_extracts" toml:"concurrent_extracts" xml:"concurrent_extracts" yaml:"concurrent_extracts"`
	Deluge             *deluge.Config `json:"deluge" toml:"deluge" xml:"deluge" yaml:"deluge"`
	Sonarr             *starr.Config  `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Radarr             *starr.Config  `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Lidarr             *starr.Config  `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
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

// UnpackerPoller stores all the running data.
type UnpackerPoller struct {
	*Flags
	*Config
	*deluge.Deluge
	*Xfers
	*SonarrQ
	*RadarrQ
	*History
	SigChan  chan os.Signal
	StopChan chan bool
}

// Xfers holds the last list of transferred pulled form Deluge.
type Xfers struct {
	sync.RWMutex
	Map map[string]*deluge.XferStatusCompat
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
