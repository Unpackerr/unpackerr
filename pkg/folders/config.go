// Package folders watches configured paths for archives to extract.
package folders

import (
	"os"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/fsnotify/fsnotify"
	"github.com/radovskyb/watcher"
	"golift.io/cnfg"
	"golift.io/xtractr"
)

// defaultPollInterval is used if Docker is detected.
const (
	DefaultPollInterval = time.Second
	MinimumPollInterval = 5 * time.Millisecond
	DefaultDeleteAfter  = 10 * time.Minute
)

// FolderConfig defines the input data for a watched folder.
//
//nolint:lll
type FolderConfig struct {
	DeleteOrig       bool           `json:"delete_original"  toml:"delete_original"   xml:"delete_original"   yaml:"delete_original"`
	DeleteFiles      bool           `json:"delete_files"     toml:"delete_files"      xml:"delete_files"      yaml:"delete_files"`
	DisableLog       bool           `json:"disable_log"      toml:"disable_log"       xml:"disable_log"       yaml:"disable_log"`
	MoveBack         bool           `json:"move_back"        toml:"move_back"         xml:"move_back"         yaml:"move_back"`
	DeleteAfter      *cnfg.Duration `json:"delete_after"     toml:"delete_after"      xml:"delete_after"      yaml:"delete_after"`
	ExtractPath      string         `json:"extract_path"     toml:"extract_path"      xml:"extract_path"      yaml:"extract_path"`
	ExtractISOs      bool           `json:"extract_isos"     toml:"extract_isos"      xml:"extract_isos"      yaml:"extract_isos"`
	DisableRecursion bool           `json:"disableRecursion" toml:"disable_recursion" xml:"disable_recursion" yaml:"disableRecursion"`
	MaxNested        int            `json:"maxNested"        toml:"max_nested"        xml:"max_nested"        yaml:"maxNested"`
	ExtrasMaxDepth   int            `json:"extrasMaxDepth"   toml:"extras_max_depth"  xml:"extras_max_depth"  yaml:"extrasMaxDepth"`
	AllowSymlinks    bool           `json:"allowSymlinks"    toml:"allow_symlinks"    xml:"allow_symlinks"    yaml:"allowSymlinks"`
	MaxBytes         string         `json:"maxBytes"         toml:"max_bytes"         xml:"max_bytes"         yaml:"maxBytes"`
	MaxFiles         int            `json:"maxFiles"         toml:"max_files"         xml:"max_files"         yaml:"maxFiles"`
	MaxRatio         float64        `json:"maxRatio"         toml:"max_ratio"         xml:"max_ratio"         yaml:"maxRatio"`
	// ResolvedMaxBytes is 0 when unset: folder watcher is uncapped.
	ResolvedMaxBytes uint64   `json:"-"             toml:"-"             xml:"-"            yaml:"-"`
	ExcludePaths     []string `json:"exclude_paths" toml:"exclude_paths" xml:"exclude_path" yaml:"exclude_paths"`
	Path             string   `json:"path"          toml:"path"          xml:"path"         yaml:"path"`
}

// WatchConfig is the undocumented folders buffer/interval settings.
type WatchConfig struct {
	Buffer   uint          `json:"buffer"   toml:"buffer"   xml:"buffer"   yaml:"buffer"`
	Interval cnfg.Duration `json:"interval" toml:"interval" xml:"interval" yaml:"interval"`
}

// Folders holds all known (created) folders in all watch paths.
type Folders struct {
	Logs
	Interval     time.Duration
	Config       []*FolderConfig
	Folders      map[string]*Folder
	Events       chan *Event
	Updates      chan *xtractr.Response
	FSNotify     *fsnotify.Watcher
	Watcher      *watcher.Watcher
	IgnoreSuffix string
}

// Logs interface for folders.
type Logs interface {
	Printf(msg string, v ...any)
	Errorf(msg string, v ...any)
	Debugf(msg string, v ...any)
}

// Folder is a "new" watched folder.
type Folder struct {
	Updated  time.Time
	Status   extract.Status
	Config   *FolderConfig
	Files    []string
	Retries  uint
	Archives xtractr.ArchiveList
	// PreFiles is the snapshot of each archive dest before extraction
	// (MoveBack only). Dest folders come from FindCompressedFiles so nested
	// archive dirs are included. Kept across retries so failed cleanups are
	// not recaptured as download content. Nil means remnant handling is skipped.
	PreFiles map[string]os.FileInfo
	// NoRetry is set when remnant_action=off leaves a blocker.
	NoRetry bool
}

// Event is a filesystem event for a watched folder.
type Event struct {
	Config *FolderConfig
	Name   string
	File   string
	Op     string
}
