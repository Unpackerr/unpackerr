package unpackerr

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
)

// syncthingStall is how long a Syncthing item may show no progress, while it is
// still smaller than the size the Starr app reports, before we stop waiting and
// extract anyway. Progress is a change in the bytes of finished files or a write
// to a Syncthing temp file. This keeps a download whose reported size is never
// reached (ignored or skipped files) from waiting forever.
const syncthingStall = 30 * time.Minute

type syncState int

const (
	syncDone    syncState = iota // every reported byte is here, or the size is unknown.
	syncWaiting                  // data is missing and still arriving.
	syncStalled                  // data is missing and nothing changed for syncthingStall.
)

// syncthingIncomplete reports whether a Syncthing item must keep waiting
// because it is smaller on disk than the Starr app reports, and logs why.
func (u *Unpackerr) syncthingIncomplete(name string, item *Extract, now time.Time) bool {
	switch have, state := syncthingProgress(item, now); state {
	case syncWaiting:
		u.Printf("[%s] Completed item still syncing: %s, %sB of %sB on disk",
			item.Label(), name, bytefmt.ByteSize(have), bytefmt.ByteSize(item.Size))
		u.setItemNote(name, item, noteWaitingSyncthing)

		return true
	case syncStalled:
		u.Printf("[%s] Completed item has %sB of %sB on disk with no change for %v, extracting anyway: %s",
			item.Label(), bytefmt.ByteSize(have), bytefmt.ByteSize(item.Size), syncthingStall, name)
	case syncDone:
	}

	return false
}

// syncthingProgress reports whether a Syncthing-delivered download is still
// missing data. The Starr app marks a download complete as soon as the remote
// client finishes, but Syncthing copies files in its own order and creates a
// temp file only for the files it is copying right now. A multi-volume archive
// can sit half-delivered with no temp file present, and extracting it then
// fails: a missing first volume, or a ratio error from missing later volumes.
// Comparing the local size to the size the Starr app reports catches that.
// Only the main loop calls this; it owns item.Size and the Sync* fields.
func syncthingProgress(item *Extract, now time.Time) (uint64, syncState) {
	if item.Size == 0 {
		return 0, syncDone // size unknown; the .tmp check is all we have.
	}

	have, active := scanSyncthingDownload(item.Path)
	if have >= item.Size {
		return have, syncDone
	}

	if item.SyncSeenAt.IsZero() || have != item.SyncSeen || !active.Equal(item.SyncActive) {
		item.SyncSeen = have
		item.SyncActive = active
		item.SyncSeenAt = now
	}

	if now.Sub(item.SyncSeenAt) < syncthingStall {
		return have, syncWaiting
	}

	return have, syncStalled
}

// scanSyncthingDownload sums the regular files under path (or path itself when
// it is a file), and returns the newest modification time of any Syncthing temp
// file. Temp files do not count as delivered: Syncthing may size a temp file to
// its final length before writing it. Their modification time shows that a large
// file is still being written even while the finished byte count stays the same.
func scanSyncthingDownload(path string) (uint64, time.Time) {
	var (
		have   uint64
		active time.Time
	)

	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // Syncthing renames files while we walk; skip what vanished.
		}

		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil //nolint:nilerr // same as above.
		}

		if strings.HasSuffix(entry.Name(), ".tmp") {
			if info.ModTime().After(active) {
				active = info.ModTime()
			}

			return nil
		}

		have += uint64(max(info.Size(), 0))

		return nil
	})

	return have, active
}
