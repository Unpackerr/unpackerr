package main

import (
	"sync"
	"time"

	"github.com/davidnewhall/unpacker-poller/deluge"
	"github.com/davidnewhall/unpacker-poller/starr"
)

// Config defines the configuration data used to start the application.
type Config struct {
	Dababase string         `json:"database" toml:"database" xml:"database" yaml:"database"` // not used.
	Interval Dur            `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Deluge   *deluge.Config `json:"deluge" toml:"deluge" xml:"deluge" yaml:"deluge"`
	Sonarr   *starr.Config  `json:"sonarr" toml:"sonarr" xml:"sonarr" yaml:"sonarr"`
	Radarr   *starr.Config  `json:"radarr" toml:"radarr" xml:"wharadarrt" yaml:"radarr"`
	Lidarr   *starr.Config  `json:"lidarr" toml:"lidarr" xml:"lidarr" yaml:"lidarr"`
	Others   []otherConfig  `json:"others" toml:"others" xml:"others" yaml:"others"` // not used.
}

// This types stores all the running data.
type runningData struct {
	Deluge  map[string]*deluge.XferStatus
	SonarrQ []*starr.SonarQueue
	RadarrQ []*starr.RadarQueue
	History map[string]extracts
	// Locks for the above maps and slices.
	hisS sync.RWMutex
	delS sync.RWMutex
	radS sync.RWMutex
	sonS sync.RWMutex
	// Only allow one extraction at a time.
	rarS sync.Mutex
}

// This type holds data for files being extracted.
type extracts struct {
	RARFile  string
	BasePath string
	App      string
	FileList []string
	Status   string
	Time     time.Time
}

// Configuration describing what to do with other tags. not used.
type otherConfig struct {
	ExtractTo    string   `json:"extract_to" toml:"extract_to" xml:"extract_to" yaml:"extract_to"`
	CreateFolder bool     `json:"create_folder" toml:"create_folder" xml:"create_folder" yaml:"create_folder"`
	Tags         []string `json:"tags" toml:"tags" xml:"tags" yaml:"tags"`
}

// Dur is used to UnmarshalTOML into a time.Duration value.
type Dur struct{ value time.Duration }

// UnmarshalTOML parses a duration type from a config file.
func (v *Dur) UnmarshalTOML(data []byte) error {
	unquoted := string(data[1 : len(data)-1])
	dur, err := time.ParseDuration(unquoted)
	if err == nil {
		v.value = dur
	}
	return err
}
