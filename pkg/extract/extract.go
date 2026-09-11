package extract

import (
	"os"
	"strings"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

// Extract holds data for files being extracted.
type Extract struct {
	Syncthing  bool
	SplitFlac  bool
	Retries    uint
	Path       string // Local path (resolved for extraction on this host).
	OutputPath string // Original path from Starr app (may be UNC/remote — used for ManualImport).
	App        starr.App
	// Name is an optional Starr instance label for logs, hooks, and the dashboard.
	// Empty uses App (Sonarr, Radarr, Folder, …). App stays the dialect for logic.
	Name        string
	URL         string
	Updated     time.Time
	DeleteDelay time.Duration
	DeleteOrig  bool
	Status      Status
	IDs         map[string]any
	Resp        *xtractr.Response
	XProg       *Progress
	// PreFiles maps cleaned full paths present in each archive dest before
	// extraction to their Lstat info (nil when the stat failed). Dest folders
	// come from FindCompressedFiles so nested archive dirs are included.
	// Snapshot once per queue item; retries must not fold in leftovers that
	// failed to clear.
	PreFiles map[string]os.FileInfo
	// NoRetry is set for limit errors, remnant_action=off, or exhausted retries.
	// EXTRACTFAILED must not re-enter the retry loop or be promoted to DELETED
	// (that bounces a still-completed Starr item back to WAITING).
	NoRetry bool
	// MaxBytes is the resolved byte cap for this Starr item (0 = unlimited).
	MaxBytes uint64
}

// Label is the human-facing instance name, or the dialect when Name is empty.
func (e *Extract) Label() string {
	if e == nil {
		return ""
	}

	if name := strings.TrimSpace(e.Name); name != "" {
		return name
	}

	return string(e.App)
}
