package unpackerr

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
)

// syncthingStall is how long a Syncthing item may show no progress, while it is
// still smaller than the size the Starr app reports, before we stop waiting and
// extract anyway. Progress is a change in the bytes of finished files, or any
// Syncthing temp file being present. This keeps a download whose reported size
// is never reached (ignored or skipped files) from waiting forever.
const syncthingStall = 30 * time.Minute

// Syncthing writes each file it is receiving to a temp file next to it and
// renames it into place when done: ".syncthing.<name>.tmp", or
// "~syncthing~<name>.tmp" on Windows. A <name> longer than
// syncthingMaxTempBase bytes is replaced by its hex SHA-256.
// See https://docs.syncthing.net/users/syncing.html#temporary-files
const (
	syncthingUnixPrefix    = ".syncthing."
	syncthingWindowsPrefix = "~syncthing~"
	syncthingTempSuffix    = ".tmp"
	syncthingMaxTempBase   = 160 - len(syncthingUnixPrefix) - len(syncthingTempSuffix)
)

// isSyncthingTemp reports whether a file name is a Syncthing temp file: one of
// the two prefixes, then a name of 1 to syncthingMaxTempBase bytes (a longer
// name is replaced by its 64 byte hash), then the .tmp suffix. Any other .tmp
// file is ordinary download content, so it counts as data and never holds the
// item back.
func isSyncthingTemp(name string) bool {
	for _, prefix := range []string{syncthingUnixPrefix, syncthingWindowsPrefix} {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, syncthingTempSuffix) {
			continue
		}

		if base := len(name) - len(prefix) - len(syncthingTempSuffix); base > 0 && base <= syncthingMaxTempBase {
			return true
		}
	}

	return false
}

// syncthingTempNames returns the two temp file names Syncthing may use, beside
// path, while it is receiving the file at path.
func syncthingTempNames(path string) []string {
	dir, base := filepath.Split(path)

	if len(base) > syncthingMaxTempBase {
		sum := sha256.Sum256([]byte(base))
		base = hex.EncodeToString(sum[:])
	}

	return []string{
		filepath.Join(dir, syncthingUnixPrefix+base+syncthingTempSuffix),
		filepath.Join(dir, syncthingWindowsPrefix+base+syncthingTempSuffix),
	}
}

// syncScan is what a Syncthing download looks like on disk right now.
// The archive search fills it through xtractr's Filter.Observe, so it costs no
// extra walk of the download folder.
type syncScan struct {
	have uint64 // bytes of finished regular files.
	temp string // a Syncthing temp file, "" when none.
	seen bool   // the search listed at least one folder entry.
}

// observe is an xtractr Filter.Observe callback. Temp files do not count as
// delivered: Syncthing may size a temp file to its final length before writing it.
func (s *syncScan) observe(path string, info os.FileInfo) {
	s.seen = true

	switch {
	case info == nil || !info.Mode().IsRegular():
	case isSyncthingTemp(filepath.Base(path)):
		s.temp = path
	default:
		s.have += uint64(max(info.Size(), 0))
	}
}

// singleFile fills the scan for a download that is a single file. The archive
// search has no folder listing to observe there, so stat the file and the two
// temp names Syncthing would use for it.
func (s *syncScan) singleFile(path string) {
	if s.seen {
		return
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return
	} else if info.Mode().IsRegular() {
		s.have = uint64(max(info.Size(), 0))
	}

	for _, temp := range syncthingTempNames(path) {
		if _, err := os.Lstat(temp); err == nil {
			s.temp = temp
			return
		}
	}
}

type syncState int

const (
	syncDone    syncState = iota // every reported byte is here, or the size is unknown.
	syncTemp                     // a Syncthing temp file exists: a file is still being copied.
	syncWaiting                  // data is missing and still arriving.
	syncStalled                  // data is missing and nothing changed for syncthingStall.
)

// syncthingWait reports whether a Syncthing item must keep waiting, and logs why.
// The Starr app marks a download complete as soon as the remote client
// finishes, but Syncthing copies files in its own order and creates a temp
// file only for the files it is copying right now. A multi-volume archive can
// sit half-delivered with no temp file present, and extracting it then fails:
// a missing first volume, or a ratio error from missing later volumes.
// So wait while any temp file exists, at any depth, and while the bytes on
// disk are short of the size the Starr app reports.
func (u *Unpackerr) syncthingWait(name string, item *Extract, scan *syncScan, now time.Time) bool {
	scan.singleFile(item.Path)

	switch state := u.lockedSyncthingProgress(item, scan, now); state {
	case syncTemp:
		u.Printf("[%s] Completed item still syncing: %s, found Syncthing .tmp file: %s", item.Label(), name, scan.temp)
	case syncWaiting:
		u.Printf("[%s] Completed item still syncing: %s, %sB of %sB on disk",
			item.Label(), name, bytefmt.ByteSize(scan.have), bytefmt.ByteSize(item.Size))
	case syncStalled:
		u.Printf("[%s] Completed item has %sB of %sB on disk with no change for %v, extracting anyway: %s",
			item.Label(), bytefmt.ByteSize(scan.have), bytefmt.ByteSize(item.Size), syncthingStall, name)

		return false
	case syncDone:
		return false
	}

	u.setItemNote(name, item, noteWaitingSyncthing)

	return true
}

// lockedSyncthingProgress runs syncthingProgress under History.mu.
// checkStarrQueue updates item.Size under that lock, and the Sync* fields are
// item state like any other that a queue snapshot may read.
func (u *Unpackerr) lockedSyncthingProgress(item *Extract, scan *syncScan, now time.Time) syncState {
	u.lockHistory()
	defer u.unlockHistory()

	return syncthingProgress(item, scan, now)
}

// syncthingProgress turns a scan into one wait decision.
// Caller must hold History.mu: this reads item.Size and writes the Sync* fields.
func syncthingProgress(item *Extract, scan *syncScan, now time.Time) syncState {
	// A temp file or a change in finished bytes is progress: restart the stall clock.
	if scan.temp != "" || item.SyncSeenAt.IsZero() || scan.have != item.SyncSeen {
		item.SyncSeen = scan.have
		item.SyncSeenAt = now
	}

	switch {
	case scan.temp != "":
		return syncTemp
	case item.Size == 0 || scan.have >= item.Size:
		return syncDone // size unknown, or every reported byte is here.
	case now.Sub(item.SyncSeenAt) < syncthingStall:
		return syncWaiting
	default:
		return syncStalled
	}
}
