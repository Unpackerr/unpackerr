package main

import (
	"strconv"
	"sync"
	"time"

	"github.com/davidnewhall/unpacker-poller/deluge"
	"github.com/davidnewhall/unpacker-poller/exp"
	"github.com/davidnewhall/unpacker-poller/starr"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Dababase string         `json:"database" toml:"database" xml:"database" yaml:"database"` // not used.
	Interval exp.Dur        `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout  exp.Dur        `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	Deluge   *deluge.Config `json:"deluge" toml:"deluge" xml:"deluge" yaml:"deluge"`
	Sonarr   *starr.Config  `json:"sonarr" toml:"sonarr" xml:"sonarr" yaml:"sonarr"`
	Radarr   *starr.Config  `json:"radarr" toml:"radarr" xml:"wharadarrt" yaml:"radarr"`
	Lidarr   *starr.Config  `json:"lidarr" toml:"lidarr" xml:"lidarr" yaml:"lidarr"`
	Others   []*OtherConfig `json:"others" toml:"others" xml:"others" yaml:"others"` // not used.
}

// ExtractStatus is our enum for an extract's status.
type ExtractStatus uint8

// Extract Statuses.
const (
	UNKNOWN = ExtractStatus(iota)
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
		return strconv.Itoa(int(status)) + " Unknown"
	}
	return []string{
		// The oder must not be be faulty.
		"Unknown", "Queued", "Extraction Progressing", "Extraction Failed",
		"Extraction Failed Twice", "Extracted, Awaiting Import", "Imported",
		"Deleting", "Delete Failed", "Deleted", "Forgotten",
	}[status]
}

// RunningData stores all the running data.
type RunningData struct {
	Deluge  map[string]*deluge.XferStatus
	SonarrQ []*starr.SonarQueue
	RadarrQ []*starr.RadarQueue
	History map[string]Extracts
	// Locks for the above maps and slices.
	hisS sync.RWMutex
	delS sync.RWMutex
	radS sync.RWMutex
	sonS sync.RWMutex
	// Only allow one extraction at a time.
	rarS sync.Mutex
}

// Extracts holds data for files being extracted.
type Extracts struct {
	RARFile  string
	BasePath string
	App      string
	FileList []string
	Status   ExtractStatus
	Updated  time.Time
}

// OtherConfig describes what to do with other tags. not used.
type OtherConfig struct {
	ExtractTo    string   `json:"extract_to" toml:"extract_to" xml:"extract_to" yaml:"extract_to"`
	CreateFolder bool     `json:"create_folder" toml:"create_folder" xml:"create_folder" yaml:"create_folder"`
	Tags         []string `json:"tags" toml:"tags" xml:"tags" yaml:"tags"`
	DeleteAfter  exp.Dur  `json:"delete_after" toml:"delete_after" xml:"delete_after" yaml:"delete_after"`
}
