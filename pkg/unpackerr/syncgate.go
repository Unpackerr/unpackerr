package unpackerr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/xtractr"
)

// syncthingStall is how long a Syncthing item may show no progress, while it is
// still smaller than the size the Starr app reports, before we extract the files
// already on disk. Progress is a change in the bytes of finished files, or any
// Syncthing temp file being present. That covers a download whose reported size
// is never reached, such as ignored or skipped files. The same window ends the
// wait for a first volume that is not on disk. An incomplete archive set the
// finder can see is not cut off here: it keeps waiting until the volumes
// arrive or the Starr app drops the download.
const syncthingStall = 30 * time.Minute

// stallWindow is the item's syncthing_stall, or the 30 minute default when
// the item has not been given one yet.
func stallWindow(item *Extract) time.Duration {
	if item != nil && item.SyncStall > 0 {
		return item.SyncStall
	}

	return syncthingStall
}

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
	have  uint64       // bytes of finished regular files.
	temp  string       // a Syncthing temp file, "" when none.
	seen  bool         // the search listed at least one folder entry.
	tails []volumeTail // continuation volumes whose first volume may still be absent.
	// unpacked is what earlier extracts of this item already handled, nil
	// when there are none. Their output is not delivered data, and a first
	// volume they unpacked counts as present for the volumes after it.
	unpacked *unpackedSet
}

// volumeTail is one continuation volume and the first-volume names that
// would make its set visible to the finder.
type volumeTail struct {
	path     string // the continuation file.
	head     string // the missing name to log when no head is present.
	alt      string // old-style .r00, which the finder treats as the head when .rar is absent.
	partStem string // headKey of "dir/stem" for .partN.rar, so any part0*1 width counts.
}

// Continuation volumes the finder does not return. .r00, .part1/.part01, and
// .7z.001 are heads. .7z.1000 is four digits and is not a volume suffix here.
var (
	rarOldContinuation  = regexp.MustCompile(`(?i)^(.+)\.r(\d{2})$`)
	rarPartContinuation = regexp.MustCompile(`(?i)^(.+)\.part(\d+)\.rar$`)
	sevenContinuation   = regexp.MustCompile(`(?i)^(.+\.7z)\.(\d{3})$`)
)

// observe is an xtractr Filter.Observe callback. Temp files do not count as
// delivered: Syncthing may size a temp file to its final length before writing it.
// Files an earlier extract wrote do not count either: the size the Starr app
// reports is the download alone, and a volume packed inside another archive
// is not a set that is still arriving.
func (s *syncScan) observe(path string, info os.FileInfo) {
	s.seen = true

	switch {
	case info == nil || !info.Mode().IsRegular():
	case isSyncthingTemp(filepath.Base(path)):
		s.temp = path
	case s.unpacked.ours(path):
	default:
		s.have += uint64(max(info.Size(), 0))

		if tail, ok := continuationVolume(path); ok {
			s.tails = append(s.tails, tail)
		}
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

		if tail, ok := continuationVolume(path); ok {
			s.tails = append(s.tails, tail)
		}
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
	syncReached syncState = iota // the bytes on disk reach the size Starr reported.
	syncUnknown                  // Starr reported no size.
	syncTemp                     // a Syncthing temp file exists: a file is still being copied.
	syncWaiting                  // data is missing and still arriving.
	syncStalled                  // data is missing and nothing changed for the stall window.
)

// syncthingWait reports whether a Syncthing item must keep waiting, and logs why.
// The Starr app marks a download complete as soon as the remote client
// finishes, but Syncthing copies files in its own order and creates a temp
// file only for the files it is copying right now. A multi-volume archive can
// sit half-delivered with no temp file present, and extracting it then fails:
// a missing first volume, or a ratio error from missing later volumes.
// So wait while any temp file exists, at any depth, and while the bytes on
// disk are short of the size the Starr app reports. An incomplete archive
// set also waits when that size is unknown, the stall window has run out,
// or a retry already has every reported byte. That wait has no time limit
// of its own. A continuation volume whose first volume is not on disk waits
// too, until nothing has changed for the stall window. A first attempt that
// has every reported byte queues without reading headers.
func (u *Unpackerr) syncthingWait(
	name string, item *Extract, scan *syncScan, files xtractr.ArchiveList, now time.Time,
) bool {
	scan.singleFile(item.Path)

	state, idle := u.lockedSyncthingProgress(item, scan, now)

	switch state {
	case syncTemp:
		u.Printf("[%s] Completed item still syncing: %s, found Syncthing .tmp file: %s", item.Label(), name, scan.temp)
		u.setItemNote(name, item, noteWaitingSyncthing)

		return true
	case syncWaiting:
		u.Printf("[%s] Completed item still syncing: %s, %sB of %sB on disk",
			item.Label(), name, bytefmt.ByteSize(scan.have), bytefmt.ByteSize(item.Size))
		u.setItemNote(name, item, noteWaitingSyncthing)

		return true
	case syncStalled, syncUnknown, syncReached:
		if u.syncthingSetShort(name, item, scan, files, state, idle) {
			u.setItemNote(name, item, noteWaitingSyncthing)

			return true
		}

		if state == syncStalled {
			u.Printf("[%s] Completed item has %sB of %sB on disk with no change for %v, extracting anyway: %s",
				item.Label(), bytefmt.ByteSize(scan.have), bytefmt.ByteSize(item.Size), stallWindow(item), name)
		}

		return false
	}

	// Every named state returns above. A state added later queues.
	return false
}

// syncthingSetShort reports an archive set that is not all on disk. A later
// volume with no first volume is found from file names alone, so that check
// runs in every state. The header check is skipped for a first attempt that
// already has every reported byte: the byte count is trusted until a retry.
// An item with earlier extracts always reads headers. A failed extract would
// mark a download as failed while its earlier output is ready to import.
func (u *Unpackerr) syncthingSetShort(
	name string, item *Extract, scan *syncScan, files xtractr.ArchiveList, state syncState, idle bool,
) bool {
	if u.waitingForVolumeHead(name, item, scan, files, idle) {
		return true
	}

	if state == syncReached && item.Retries == 0 && !hasUnpacked(item) {
		return false
	}

	_, err := u.incompleteSet(item, files)
	if err == nil {
		return false
	}

	u.Printf("[%s] Completed item still syncing: %s, archive set not complete: %v", item.Label(), name, err)

	return true
}

// waitingForVolumeHead reports a continuation volume whose first volume is not
// on disk, while the download is still changing. None of the archives that were
// found needs that volume, so once nothing has changed for the stall window they
// are extracted. A first volume that arrives after that is found on a later pass.
func (u *Unpackerr) waitingForVolumeHead(
	name string, item *Extract, scan *syncScan, files xtractr.ArchiveList, idle bool,
) bool {
	tail, head := missingVolumeHead(scan, files)
	if tail == "" {
		return false
	}

	if idle {
		u.Printf("[%s] Completed item has no %s for %s after %v with no change, not waiting for it: %s",
			item.Label(), head, tail, stallWindow(item), name)

		return false
	}

	u.Printf("[%s] Completed item still syncing: %s, archive set not complete: %s: missing %s",
		item.Label(), name, tail, head)

	return true
}

// continuationVolume reports a volume the finder skips because it is not the
// head of its set. The head names are what a later poll needs to see.
func continuationVolume(path string) (volumeTail, bool) {
	base := filepath.Base(path)
	dir := filepath.Dir(path)

	if match := rarOldContinuation.FindStringSubmatch(base); match != nil {
		if match[2] == "00" {
			return volumeTail{}, false
		}

		return volumeTail{
			path: path,
			head: filepath.Join(dir, match[1]+".rar"),
			alt:  filepath.Join(dir, match[1]+".r00"),
		}, true
	}

	if match := rarPartContinuation.FindStringSubmatch(base); match != nil {
		if partOneVolume(match[2]) {
			return volumeTail{}, false
		}

		headNum := fmt.Sprintf("%0*d", len(match[2]), 1)

		return volumeTail{
			path:     path,
			head:     filepath.Join(dir, match[1]+".part"+headNum+".rar"),
			partStem: headKey(filepath.Join(dir, match[1])),
		}, true
	}

	if match := sevenContinuation.FindStringSubmatch(base); match != nil {
		if match[2] == "001" {
			return volumeTail{}, false
		}

		return volumeTail{path: path, head: filepath.Join(dir, match[1]+".001")}, true
	}

	return volumeTail{}, false
}

// partOneVolume reports a .partN.rar index that the finder treats as the
// first volume: 1 with any number of leading zeros.
func partOneVolume(digits string) bool {
	return strings.TrimLeft(digits, "0") == "1"
}

// missingVolumeHead returns a continuation path and the head name to log
// when that head is not in this find and an earlier extract did not unpack it.
func missingVolumeHead(scan *syncScan, files xtractr.ArchiveList) (string, string) {
	if scan == nil || len(scan.tails) == 0 {
		return "", ""
	}

	found := make(map[string]struct{}, files.Count())
	for _, path := range files.List() {
		found[headKey(path)] = struct{}{}
	}

	var unpacked map[string]struct{}
	if scan.unpacked != nil {
		unpacked = scan.unpacked.heads
	}

	for _, tail := range scan.tails {
		if volumeHeadFound(found, tail) || volumeHeadFound(unpacked, tail) {
			continue
		}

		return tail.path, tail.head
	}

	return "", ""
}

// volumeHeadFound reports whether heads, a set of headKey values, holds a
// first volume for tail.
func volumeHeadFound(heads map[string]struct{}, tail volumeTail) bool {
	if _, ok := heads[headKey(tail.head)]; ok {
		return true
	}

	if tail.alt != "" {
		if _, ok := heads[headKey(tail.alt)]; ok {
			return true
		}
	}

	if tail.partStem == "" {
		return false
	}

	prefix := tail.partStem + ".part"

	for key := range heads {
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok || !strings.HasSuffix(rest, ".rar") {
			continue
		}

		if partOneVolume(strings.TrimSuffix(rest, ".rar")) {
			return true
		}
	}

	return false
}

// headKey is an archive path in the form used to match a continuation volume
// to its first volume. The finder matches those names without regard to case,
// so the file name is folded. The folder is compared as written: on a
// case-sensitive filesystem it can be a different folder.
func headKey(path string) string {
	dir, base := filepath.Split(filepath.Clean(path))

	return dir + strings.ToLower(base)
}

// incompleteSet returns the first archive set whose volumes are not all on disk.
// Any other header error is left for the extract to report.
func (u *Unpackerr) incompleteSet(item *Extract, files xtractr.ArchiveList) (string, error) {
	password := u.getPasswordFromPath(item.Path)

	for _, archive := range files.List() {
		err := xtractr.CheckVolumes(&xtractr.XFile{
			FilePath:  archive,
			Password:  password,
			Passwords: []string(u.Passwords),
		})
		if errors.Is(err, xtractr.ErrMissingVolume) {
			return archive, fmt.Errorf("%s: %w", archive, err)
		}
	}

	return "", nil
}

// lockedSyncthingProgress runs syncthingProgress under History.mu.
// checkStarrQueue updates item.Size under that lock, and the Sync* fields are
// item state like any other that a queue snapshot may read. idle is true when
// nothing changed for the stall window, whatever the state: no finished bytes
// arrived and no Syncthing temp file was seen.
func (u *Unpackerr) lockedSyncthingProgress(item *Extract, scan *syncScan, now time.Time) (syncState, bool) {
	u.lockHistory()
	defer u.unlockHistory()

	state := syncthingProgress(item, scan, now)

	return state, now.Sub(item.SyncSeenAt) >= stallWindow(item)
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
	case item.Size == 0:
		return syncUnknown
	case scan.have >= item.Size:
		return syncReached
	case now.Sub(item.SyncSeenAt) < stallWindow(item):
		return syncWaiting
	default:
		return syncStalled
	}
}
