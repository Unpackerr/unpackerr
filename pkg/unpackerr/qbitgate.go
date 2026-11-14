package unpackerr

import (
	"context"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/starr"
)

const (
	noteWaitingFiles = "waiting for download files"
	// qbitLogEvery limits progress lines for an item that is still missing files.
	qbitLogEvery = 10 * time.Minute
)

type qbitState int

const (
	qbitUnused  qbitState = iota // no qBittorrent answer; use the other checks.
	qbitWaiting                  // a selected file is missing, short, or still syncing.
	qbitReady                    // every selected file is in place.
)

// waitForDelivery reports whether a completed download must keep waiting for its
// files to land locally. With a qBittorrent configured it compares the torrent's
// file list with the disk. Without one, or if it does not answer, the Syncthing
// checks (temp file, then size) apply as before.
func (u *Unpackerr) waitForDelivery(name string, item *Extract, now time.Time) bool {
	if item.Syncthing {
		if tmpFile := u.hasSyncThingFile(item.Path); tmpFile != "" {
			u.Printf("[%s] Completed item still syncing: %s, found Syncthing .tmp file: %s", item.Label(), name, tmpFile)
			u.setItemNote(name, item, noteWaitingSyncthing)

			return true
		}
	}

	switch u.qbitCheck(name, item, now) {
	case qbitWaiting:
		return true
	case qbitReady:
		return false
	case qbitUnused:
	}

	return item.Syncthing && u.syncthingIncomplete(name, item, now)
}

// qbitCheck asks the item's qBittorrent for the torrent's file list and checks that
// every selected file exists under the download at exactly its listed size, and that
// no Syncthing temp file remains anywhere under the download. There is no timeout:
// a missing file is a fact, not a guess. Any failure to get a usable list returns
// qbitUnused so the caller falls back to the other checks.
func (u *Unpackerr) qbitCheck(name string, item *Extract, now time.Time) qbitState {
	conf := u.starrConfFor(item)
	if conf == nil || conf.QbitURL == "" {
		return qbitUnused
	}

	hash := torrentHash(item.IDs["downloadId"])
	if hash == "" {
		return qbitUnused // not a torrent, or the Starr app did not say which one.
	}

	timeout := conf.Timeout.Duration
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	files, err := conf.qbitClient().files(ctx, hash)
	if err != nil {
		u.qbitUnusable(name, item, now, err.Error())
		return qbitUnused
	}

	check, ok := checkTorrentFiles(item.Path, files)
	if !ok {
		u.qbitUnusable(name, item, now, "file list does not match the download folder name")
		return qbitUnused
	}

	item.SyncSeenAt = time.Time{} // a later fallback to the size check starts a fresh window.

	if check.missing == 0 && check.temp == "" {
		if item.Note == noteWaitingFiles || item.Note == noteWaitingSyncthing {
			u.Printf("[%s] Completed item has all %d files from qBittorrent in place: %s", item.Label(), check.files, name)
		}

		return qbitReady
	}

	if item.Note != noteWaitingFiles || now.Sub(item.QbitLogAt) >= qbitLogEvery {
		item.QbitLogAt = now

		waitingOn := "Syncthing temp file " + check.temp
		if check.missing > 0 {
			waitingOn = check.firstMissing
		}

		u.Printf("[%s] Completed item still arriving: %s, %d of %d files in place (%sB of %sB), waiting on: %s",
			item.Label(), name, check.files-check.missing, check.files,
			bytefmt.ByteSize(check.have), bytefmt.ByteSize(check.want), waitingOn)
	}

	u.setItemNote(name, item, noteWaitingFiles)

	return qbitWaiting
}

// qbitUnusable logs, at most every qbitLogEvery per item, why the qBittorrent check was skipped.
func (u *Unpackerr) qbitUnusable(name string, item *Extract, now time.Time, why string) {
	if now.Sub(item.QbitLogAt) < qbitLogEvery {
		return
	}

	item.QbitLogAt = now
	u.Printf("[%s] qBittorrent file check unavailable for %s (%s); using the other checks", item.Label(), name, why)
}

type torrentCheck struct {
	files, missing int
	have, want     uint64
	firstMissing   string
	temp           string
}

// checkTorrentFiles compares qBittorrent's file list with the disk. Names are relative
// to the save path and start with the torrent's root folder (or are the file name for a
// single-file torrent), so they resolve against the parent of the download path. ok is
// false when that mapping does not hold, because then nothing here can be trusted.
func checkTorrentFiles(downloadPath string, files []qbitFile) (torrentCheck, bool) {
	var check torrentCheck

	root := filepath.Base(downloadPath)
	base := filepath.Dir(downloadPath)

	for _, file := range files {
		if file.Priority <= 0 {
			continue // not selected for download; it never arrives.
		}

		rel := filepath.FromSlash(file.Name)
		if !filepath.IsLocal(rel) || strings.SplitN(rel, string(filepath.Separator), 2)[0] != root { //nolint:mnd
			return check, false
		}

		check.files++
		check.want += uint64(max(file.Size, 0))

		info, err := os.Stat(filepath.Join(base, rel))
		if err == nil && info.Mode().IsRegular() && info.Size() == file.Size {
			check.have += uint64(max(file.Size, 0))
			continue
		}

		if check.missing == 0 {
			check.firstMissing = file.Name
		}

		check.missing++
	}

	if check.files == 0 {
		return check, false
	}

	check.temp = findSyncthingTemp(downloadPath)

	return check, true
}

// findSyncthingTemp returns the first Syncthing temp file under path, at any depth.
// Syncthing names them .syncthing.<name>.tmp, or ~syncthing~<name>.tmp on Windows.
func findSyncthingTemp(path string) string {
	var found string

	_ = filepath.WalkDir(path, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // Syncthing renames files while we walk; skip what vanished.
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".tmp") &&
			(strings.HasPrefix(name, ".syncthing.") || strings.HasPrefix(name, "~syncthing~")) {
			found = file
			return fs.SkipAll
		}

		return nil
	})

	return found
}

// torrentHash returns a lower-case torrent info hash (v1 or v2) from a Starr download ID, or "".
func torrentHash(id any) string {
	hash, _ := id.(string)
	hash = strings.ToLower(strings.TrimSpace(hash))

	if len(hash) != 40 && len(hash) != 64 { // SHA-1 (v1) and SHA-256 (v2) in hex.
		return ""
	}

	if _, err := hex.DecodeString(hash); err != nil {
		return ""
	}

	return hash
}

// starrConfFor returns the configured Starr instance an item came from, or nil.
func (u *Unpackerr) starrConfFor(item *Extract) *StarrConfig {
	switch item.App {
	case starr.Lidarr:
		return starrConfByURL[LidarrConfig, *LidarrConfig](u.Lidarr, item.URL)
	case starr.Radarr:
		return starrConfByURL[RadarrConfig, *RadarrConfig](u.Radarr, item.URL)
	case starr.Readarr:
		return starrConfByURL[ReadarrConfig, *ReadarrConfig](u.Readarr, item.URL)
	case starr.Sonarr:
		return starrConfByURL[SonarrConfig, *SonarrConfig](u.Sonarr, item.URL)
	default:
		return nil
	}
}

func starrConfByURL[T any, P starrApp[T]](list InstanceMap[T], url string) *StarrConfig {
	for _, item := range instanceValues(list) {
		if conf := asStarr[T, P](item).conf(); conf.URL == url {
			return conf
		}
	}

	return nil
}

// qbitClient returns the instance's qBittorrent client, rebuilt when the settings change.
func (c *StarrConfig) qbitClient() *qbitClient {
	if q := c.qbit; q != nil && q.url == c.QbitURL && q.user == c.QbitUser && q.pass == c.QbitPass {
		return q
	}

	c.qbit = newQbitClient(c.QbitURL, c.QbitUser, c.QbitPass)

	return c.qbit
}
