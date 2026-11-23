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
	Syncthing bool
	// SplitFlac splits FLAC and APE images referenced by a CUE sheet.
	SplitFlac bool
	// APEFormat is ape, wav, or flac. Empty means ape.
	APEFormat xtractr.AudioFormat
	// APECompression is the Monkey's Audio level used when APEFormat is ape.
	// A default of 0 is set to 2000.
	APECompression int
	Retries        uint
	// HookFail is how many webhook or command-hook deliveries failed for this extract.
	HookFail uint
	// HookMessages maps webhook/cmdhook instance slug to Discord message id or Telegram message_id.
	HookMessages map[string]string
	Path         string // Local path (resolved for extraction on this host).
	OutputPath   string // Original path from Starr app (may be UNC/remote — used for ManualImport).
	App          starr.App
	// Name is an optional Starr instance label for logs, hooks, and the dashboard.
	// Empty uses App (Sonarr, Radarr, Folder, …). App stays the dialect for logic.
	Name        string
	URL         string
	Updated     time.Time
	DeleteDelay time.Duration
	DeleteOrig  bool
	Status      Status
	// Due is the next status-timer deadline. The main loop stamps it so HTTP
	// queue snapshots do not read live config or the folder tracker.
	Due     time.Time
	DueKind string
	IDs     map[string]any
	Resp    *xtractr.Response
	XProg   *Progress
	// Note is a short waiting-state caption for the live queue (e.g. no archives yet).
	Note string
	// Event is how an item arrived: fsnotify, polling, later manual. Starr omits it for now.
	Event string
	// PreFiles maps cleaned full paths present in each archive dest before
	// extraction to their Lstat info (nil when the stat failed). Dest folders
	// come from FindCompressedFiles so nested archive dirs are included.
	// Snapshot once per queue item; retries must not fold in leftovers that
	// failed to clear. A finished Syncthing extract drops it: the item may be
	// extracted again for a late archive, and that extract needs a snapshot
	// that holds this one's output.
	PreFiles map[string]os.FileInfo
	// Unpacked lists the archives that finished extracts already unpacked for
	// a Syncthing item: cleaned paths, every volume. A later search skips
	// them in every status, so a retry or a late archive never unpacks one
	// twice. It is emptied when the extracted files are deleted.
	Unpacked []string
	// OutputFiles lists the files inside the folders those extracts moved
	// into the download, as they were when each extract ended. Resp.NewFiles
	// names such a folder once. A file Syncthing adds to it later is not
	// listed here, so it still counts as delivered and can be extracted.
	OutputFiles []string
	// NoRetry is set for limit errors, remnant_action=off, or exhausted retries.
	// EXTRACTFAILED must not re-enter the retry loop or be promoted to DELETED
	// (that bounces a still-completed Starr item back to WAITING).
	NoRetry bool
	// MaxBytes is the resolved byte cap for this Starr item (0 = unlimited).
	MaxBytes uint64
	// Size is the download size the Starr app reports for this item (0 = unknown).
	// Every queue poll refreshes it. With Syncthing enabled, extraction waits
	// until this many bytes are on disk.
	Size uint64
	// SyncSeen is the finished-file byte count last measured for a Syncthing
	// item, and SyncSeenAt when it last changed or a Syncthing temp file was
	// last seen. Used to tell a slow sync from a stalled one.
	SyncSeen   uint64
	SyncSeenAt time.Time
	// SyncStall is how long this item may show no progress before a short
	// download is extracted anyway. Zero uses the 30 minute default. Every
	// queue poll refreshes it from the instance's syncthing_stall.
	SyncStall time.Duration
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
