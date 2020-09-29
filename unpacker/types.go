package unpacker

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

const (
	defaultTimeout     = 10 * time.Second
	minimumInterval    = 15 * time.Second
	defaultRetryDelay  = 5 * time.Minute
	defaultStartDelay  = time.Minute
	minimumDeleteDelay = time.Second
	torrent            = "torrent"
	completed          = "Completed"
	mebiByte           = 1024 * 1024
	suffix             = "_unpackerred" // suffix for unpacked folders.
	updateChanSize     = 1000           // Size of update channel. This is sufficiently large.
	defaultQueueSize   = 20000          // Channel queue size for file system events.
	minimumQueueSize   = 1000
)

// Config defines the configuration data used to start the application.
type Config struct {
	Debug       bool            `json:"debug" toml:"debug" xml:"debug" yaml:"debug"`
	Quiet       bool            `json:"quiet" toml:"quiet" xml:"quiet" yaml:"quiet"`
	Parallel    uint            `json:"parallel" toml:"parallel" xml:"parallel" yaml:"parallel"`
	LogFile     string          `json:"log_file" toml:"log_file" xml:"log_file" yaml:"log_file"`
	Interval    cnfg.Duration   `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
	Timeout     cnfg.Duration   `json:"timeout" toml:"timeout" xml:"timeout" yaml:"timeout"`
	DeleteDelay cnfg.Duration   `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	StartDelay  cnfg.Duration   `json:"start_delay" toml:"start_delay" xml:"start_delay" yaml:"start_delay"`
	RetryDelay  cnfg.Duration   `json:"retry_delay" toml:"retry_delay" xml:"retry_delay" yaml:"retry_delay"`
	Buffer      int             `json:"buffer" toml:"buffer" xml:"buffer" yaml:"buffer"`
	Sonarr      []*sonarrConfig `json:"sonarr,omitempty" toml:"sonarr" xml:"sonarr" yaml:"sonarr,omitempty"`
	Radarr      []*radarrConfig `json:"radarr,omitempty" toml:"radarr" xml:"radarr" yaml:"radarr,omitempty"`
	Lidarr      []*lidarrConfig `json:"lidarr,omitempty" toml:"lidarr" xml:"lidarr" yaml:"lidarr,omitempty"`
	Folders     []*folderConfig `json:"folder,omitempty" toml:"folder" xml:"folder" yaml:"folder,omitempty"`
}

type radarrConfig struct {
	*starr.Config
	Path         string              `json:"path" toml:"path" xml:"path" yaml:"path"`
	Queue        []*starr.RadarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type sonarrConfig struct {
	*starr.Config
	Path         string              `json:"path" toml:"path" xml:"path" yaml:"path"`
	Queue        []*starr.SonarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type lidarrConfig struct {
	*starr.Config
	Path         string                `json:"path" toml:"path" xml:"path" yaml:"path"`
	Queue        []*starr.LidarrRecord `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

type folderConfig struct {
	DeleteOrig  bool          `json:"delete_original" toml:"delete_original" xml:"delete_original" yaml:"delete_original"`
	MoveBack    bool          `json:"move_back" toml:"move_back" xml:"move_back" yaml:"move_back"`
	DeleteAfter cnfg.Duration `json:"delete_after" toml:"delete_after" xml:"delete_after" yaml:"delete_after"`
	Path        string        `json:"path" toml:"path" xml:"path" yaml:"path"`
}

// ExtractStatus is our enum for an extract's status.
type ExtractStatus uint8

// Extract Statuses.
const (
	WAITING = ExtractStatus(iota)
	QUEUED
	EXTRACTING
	EXTRACTFAILED
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
		"Waiting, pre-Queue", "Queued", "Extraction Progressing", "Extraction Failed",
		"Extracted, Awaiting Import", "Imported",
		"Deleting", "Delete Failed", "Deleted",
	}[status]
}

// Use in r.eCount to return activity counters.
type eCounters struct {
	waiting    uint
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
	EnvPrefix  string
}

// Folders holds all known (created) folders in all watch paths.
type Folders struct {
	Config  []*folderConfig
	Folders map[string]*Folder
	Events  chan *eventData
	Updates chan *update
	Logf    func(msg string, v ...interface{})
	Debug   func(msg string, v ...interface{})
	Watcher *fsnotify.Watcher
}

// Unpackerr stores all the running data.
type Unpackerr struct {
	*Flags
	*Config
	*History
	*xtractr.Xtractr
	folders *Folders
	sigChan chan os.Signal
	updates chan *Extracts // external updates coming in
	log     *log.Logger
}

// History holds the history of extracted items.
type History struct {
	Finished  uint
	Restarted uint
	Map       map[string]*Extracts
}

// Extracts holds data for files being extracted.
type Extracts struct {
	Path    string
	App     string
	Files   []string
	Status  ExtractStatus
	Updated time.Time
}

// Folder is a "new" watched folder.
type Folder struct {
	last time.Time
	step ExtractStatus
	cnfg *folderConfig
	list []string
}

type eventData struct {
	cnfg *folderConfig
	name string
	file string
}

type update struct {
	Step ExtractStatus
	Name string
	Resp *xtractr.Response
}
