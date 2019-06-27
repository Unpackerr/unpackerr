package unpackerpoller

import (
	"sync"
	"time"

	"github.com/golift/deluge"
	"github.com/golift/starr"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Interval           Duration     `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout            Duration     `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay        Duration     `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	ConcurrentExtracts int          `json:"concurrent_extracts" toml:"concurrent_extracts" xml:"concurrent_extracts" yaml:"concurrent_extracts"`
	Deluge             delugeConfig `json:"deluge" toml:"deluge" xml:"deluge" yaml:"deluge"`
	Sonarr             starrConfig  `json:"sonarr" toml:"sonarr" xml:"sonarr" yaml:"sonarr"`
	Radarr             starrConfig  `json:"radarr" toml:"radarr" xml:"wharadarrt" yaml:"radarr"`
	Lidarr             starrConfig  `json:"lidarr" toml:"lidarr" xml:"lidarr" yaml:"lidarr"`
	deluge             deluge.Config
	radarr             *starr.Config
	sonarr             *starr.Config
}

type starrConfig struct {
	APIKey   string   `json:"api_key" toml:"api_key" xml:"api_key" yaml:"api_key"`
	URL      string   `json:"url" toml:"url" xml:"url" yaml:"url"`
	HTTPPass string   `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string   `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Timeout  Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
}

type delugeConfig struct {
	URL      string   `json:"url" toml:"url" xml:"url" yaml:"url"`
	Password string   `json:"password" toml:"password" xml:"password" yaml:"password"`
	HTTPPass string   `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string   `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
	Timeout  Duration `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
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
	DELETEFAILED
	DELETED
	FORGOTTEN
)

// String makes ExtractStatus human readable.
func (status ExtractStatus) String() string {
	if status > FORGOTTEN {
		return "Unknown"
	}
	return []string{
		// The order must not be be faulty.
		"Missing", "Queued", "Extraction Progressing", "Extraction Failed",
		"Extraction Failed Twice", "Extracted, Awaiting Import", "Imported",
		"Deleting", "Delete Failed", "Deleted", "Forgotten",
	}[status]
}

// Use in r.eCount to return activity counters.
type eCounters struct {
	queued     int
	extracting int
	failed     int
	extracted  int
	imported   int
	deleted    int
}

// RunningData stores all the running data.
type RunningData struct {
	DeleteDelay time.Duration
	Deluge      map[string]*deluge.XferStatus
	SonarrQ     []*starr.SonarQueue
	RadarrQ     []*starr.RadarQueue
	History     map[string]Extracts
	// Locks for the above maps and slices.
	hisS sync.RWMutex
	delS sync.RWMutex
	radS sync.RWMutex
	sonS sync.RWMutex
	// Only allow N extractions at a time.
	maxExtracts int
}

// Extracts holds data for files being extracted.
type Extracts struct {
	Path    string
	App     string
	Files   []string
	Status  ExtractStatus
	Updated time.Time
}

// Duration is used to UnmarshalTOML into a time.Duration value.
type Duration struct {
	time.Duration
}

// UnmarshalTOML parses a duration type from a config file.
func (v *Duration) UnmarshalTOML(data []byte) error {
	unquoted := string(data[1 : len(data)-1])
	dur, err := time.ParseDuration(unquoted)
	if err == nil {
		v.Duration = dur
	}
	return err
}

// UnmarshalJSON parses a duration type from a config file.
func (v *Duration) UnmarshalJSON(data []byte) error {
	return v.UnmarshalTOML(data)
}
