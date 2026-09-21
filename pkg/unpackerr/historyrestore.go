package unpackerr

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/folders"
	"golift.io/starr"
	"golift.io/xtractr"
)

// restoreQueueFromHistory copies recent JSONL rows into History.Map so a crash
// or kill does not lose Starr import-wait / delete-delay or folder delete-after
// / retry work. Folder items land in Map here; seedFolderTracker (after
// PollFolders) attaches them to the watch tracker. Call after validateApps so
// URL→dialect matching sees the live Starr list. Does not take histMu and
// History.mu at the same time (updateQueueStatus holds History.mu then histMu).
func (u *Unpackerr) restoreQueueFromHistory() {
	if u.KeepHistory == 0 {
		return
	}

	u.histMu.Lock()
	rows := append([]HistoryRecord(nil), u.records...)
	u.histMu.Unlock()

	if len(rows) == 0 {
		return
	}

	now := time.Now()
	cutoff := now.Add(-historyRestoreAge)
	restored := 0

	u.lockHistory()
	defer u.unlockHistory()

	for _, rec := range rows {
		itemID := rec.ID
		if itemID == "" {
			itemID = rec.Path
		}

		if rec.Forgotten {
			if itemID != "" && rec.Kind != FolderString {
				u.forgotten[itemID] = struct{}{}
			}

			continue
		}

		item, itemID, ok := u.extractFromHistory(rec, now, cutoff)
		if !ok {
			continue
		}

		if _, exists := u.Map[itemID]; exists {
			continue
		}

		if item.App == FolderString && !u.seedFolderItemLocked(itemID, item, false) {
			continue
		}

		u.Map[itemID] = item
		u.stampQueueDue(itemID, item)

		restored++
	}

	if restored > 0 {
		u.Printf("[Unpackerr] Restored %d queue item(s) from history (last %s).", restored, historyRestoreAge)
	}
}

func (u *Unpackerr) extractFromHistory(rec HistoryRecord, now, cutoff time.Time) (*Extract, string, bool) {
	itemID, stamp, status, errMsg, kind, ok := u.historyRestoreGate(rec, cutoff)
	if !ok {
		return nil, "", false
	}

	return u.extractFromHistoryRecord(rec, kind, status, errMsg, stamp, now), itemID, true
}

func (u *Unpackerr) historyRestoreGate(
	rec HistoryRecord,
	cutoff time.Time,
) (string, time.Time, ExtractStatus, string, string, bool) {
	itemID := rec.ID
	if itemID == "" {
		itemID = rec.Path
	}

	if itemID == "" {
		return "", time.Time{}, 0, "", "", false
	}

	stamp := historyStamp(rec)
	if stamp.IsZero() || stamp.Before(cutoff) {
		return "", time.Time{}, 0, "", "", false
	}

	status, errMsg, keep := restoreQueueStatus(rec.Status)
	if !keep {
		return "", time.Time{}, 0, "", "", false
	}

	kind := u.historyKind(rec)
	// Saved WAITING is folder-only (Starr poll recreates WAITING). QUEUED still
	// maps to WAITING above and must restore for Starr titles.
	if rec.Status == WAITING && kind != FolderString {
		return "", time.Time{}, 0, "", "", false
	}

	if restoreNeedsKind(status) && kind == "" {
		return "", time.Time{}, 0, "", "", false
	}

	if kind == FolderString && !restoreFolderStatus(status) {
		return "", time.Time{}, 0, "", "", false
	}

	return itemID, stamp, status, errMsg, kind, true
}

func (u *Unpackerr) extractFromHistoryRecord(
	rec HistoryRecord,
	kind string,
	status ExtractStatus,
	errMsg string,
	stamp, now time.Time,
) *Extract {
	item := &Extract{
		Syncthing:  rec.Syncthing,
		SplitFlac:  rec.SplitFlac,
		Retries:    rec.Retries,
		HookFail:   rec.HookFail,
		Path:       rec.Path,
		OutputPath: rec.OutputPath,
		App:        starr.App(kind),
		URL:        rec.URL,
		Updated:    rec.Updated,
		DeleteOrig: rec.DeleteOrig,
		Status:     status,
		NoRetry:    rec.NoRetry,
		MaxBytes:   rec.MaxBytes,
		PreFiles:   preFilesFromKeys(rec.PreFiles),
		IDs:        cloneIDs(rec.IDs),
		Event:      rec.Event,
	}

	if rec.App != "" && rec.App != kind {
		item.Name = rec.App
	}

	applyHistoryRestoreDelay(item, rec.DeleteDelay, u.DeleteDelay.Duration)
	applyHistoryRestoreResp(item, rec, errMsg)
	applyHistoryRestoreClock(item, rec.Status, now, stamp, u.RetryDelay.Duration)
	item.XProg = &ExtractProgress{Extract: item}

	return item
}

func applyHistoryRestoreDelay(item *Extract, delay string, fallback time.Duration) {
	if delay != "" {
		if parsed, err := time.ParseDuration(delay); err == nil {
			item.DeleteDelay = parsed
		}
	}

	if item.DeleteDelay == 0 {
		item.DeleteDelay = fallback
	}
}

func applyHistoryRestoreResp(item *Extract, rec HistoryRecord, errMsg string) {
	if rec.Error != "" {
		errMsg = rec.Error
	}

	if len(rec.NewFiles) == 0 && rec.Bytes == 0 && errMsg == "" &&
		len(rec.OrigFiles) == 0 && rec.Output == "" && rec.Queue == 0 &&
		rec.Elapsed == "" && len(rec.ExtraFiles) == 0 {
		return
	}

	item.Resp = &xtractr.Response{
		NewFiles: append([]string(nil), rec.NewFiles...),
		Size:     rec.Bytes,
		Output:   rec.Output,
		Queued:   rec.Queue,
		Started:  rec.Started,
	}

	if rec.Elapsed != "" {
		if parsed, err := time.ParseDuration(rec.Elapsed); err == nil {
			item.Resp.Elapsed = parsed
		}
	}

	if len(rec.OrigFiles) > 0 {
		item.Resp.Archives = xtractr.ArchiveList{"": append([]string(nil), rec.OrigFiles...)}
	}

	if len(rec.ExtraFiles) > 0 {
		item.Resp.Extras = xtractr.ArchiveList{"": append([]string(nil), rec.ExtraFiles...)}
	}

	if rec.Error != "" {
		item.Resp.Error = errors.New(rec.Error) //nolint:err113 // stored pipeline error
		return
	}

	if errMsg != "" {
		item.Resp.Error = errInterruptedRestart
	}
}

func applyHistoryRestoreClock(item *Extract, saved ExtractStatus, now, stamp time.Time, retryDelay time.Duration) {
	switch item.Status {
	case EXTRACTED, WAITING:
		item.Updated = now
	case EXTRACTFAILED:
		if !item.NoRetry && (saved == EXTRACTING || saved == DELETING) {
			item.Updated = now.Add(-retryDelay)
			if item.Resp == nil {
				item.Resp = &xtractr.Response{Error: errInterruptedRestart}
			}
		}
	}

	if item.Updated.IsZero() {
		item.Updated = stamp
	}
}

func restoreQueueStatus(status ExtractStatus) (ExtractStatus, string, bool) {
	switch status {
	case WAITING, EXTRACTED, IMPORTED, EXTRACTFAILED, EXTRACTEDNOTHING:
		return status, "", true
	case QUEUED:
		return WAITING, "", true
	case EXTRACTING, DELETING:
		return EXTRACTFAILED, errInterruptedRestart.Error(), true
	case DELETEFAILED:
		return IMPORTED, "", true
	default:
		return status, "", false
	}
}

func restoreNeedsKind(status ExtractStatus) bool {
	switch status {
	case EXTRACTED, WAITING, EXTRACTFAILED, EXTRACTEDNOTHING:
		return true
	default:
		return false
	}
}

func restoreFolderStatus(status ExtractStatus) bool {
	switch status {
	case WAITING, EXTRACTFAILED, EXTRACTED, EXTRACTEDNOTHING:
		return true
	default:
		return false
	}
}

func historyStamp(rec HistoryRecord) time.Time {
	switch {
	case !rec.Updated.IsZero():
		return rec.Updated
	case !rec.Finished.IsZero():
		return rec.Finished
	default:
		return rec.Started
	}
}

func (u *Unpackerr) historyKind(rec HistoryRecord) string {
	if rec.Kind != "" {
		return rec.Kind
	}

	switch rec.App {
	case FolderString, string(starr.Sonarr), string(starr.Radarr),
		string(starr.Lidarr), string(starr.Readarr):
		return rec.App
	}

	return string(u.kindFromURL(rec.URL))
}

// seedFolderTracker attaches restored Folder items to the watch tracker.
// PollFolders replaces u.folders, so this must run after that. requireConfig
// is true here: a watch list that does not contain the path drops the item.
func (u *Unpackerr) seedFolderTracker() {
	if u.folders == nil {
		return
	}

	u.lockHistory()
	defer u.unlockHistory()

	dropped := false

	for itemID, item := range u.Map {
		if item == nil || item.App != FolderString {
			continue
		}

		if u.seedFolderItemLocked(itemID, item, true) {
			continue
		}

		delete(u.Map, itemID)

		dropped = true
	}

	if dropped {
		u.notifyQueueLocked()
	}
}

// seedFolderItemLocked copies one restored Folder extract onto the tracker.
// When requireConfig is false and the watcher has no configs yet (Start, before
// PollFolders), the item stays in Map for seedFolderTracker. Caller holds History.mu.
func (u *Unpackerr) seedFolderItemLocked(itemID string, item *Extract, requireConfig bool) bool {
	if u.folders == nil {
		return !requireConfig
	}

	if _, exists := u.folders.Folders[itemID]; exists {
		return true
	}

	if len(u.folders.Config) == 0 {
		return !requireConfig
	}

	cfg := folderConfigForPath(u.folders.Config, item.Path)
	if cfg == nil {
		u.Debugf("[Folder] Not restoring %s: no matching watch path", itemID)

		return false
	}

	u.folders.Folders[itemID] = folderFromExtract(item, cfg)

	if item.Event == "" {
		item.Event = folders.KindFSNotify
		if cfg.UsesPoller() {
			item.Event = folders.KindPolling
		}
	}

	return true
}

func folderFromExtract(item *Extract, cfg *FolderConfig) *Folder {
	folder := &Folder{
		Updated:  item.Updated,
		Status:   item.Status,
		Config:   cfg,
		Retries:  item.Retries,
		NoRetry:  item.NoRetry,
		PreFiles: item.PreFiles,
	}

	if item.Resp == nil {
		return folder
	}

	folder.Files = append([]string(nil), item.Resp.NewFiles...)
	folder.Archives = item.Resp.Archives

	return folder
}

func folderConfigForPath(configs []*FolderConfig, itemPath string) *FolderConfig {
	itemPath = filepath.Clean(itemPath)

	var (
		best    *FolderConfig
		bestLen = -1
	)

	for _, cfg := range configs {
		if cfg == nil || cfg.Path == "" || cfg.IsExcludedPath(itemPath) {
			continue
		}

		clean := filepath.Clean(cfg.Path)
		if !folderPathContains(clean, itemPath) {
			continue
		}

		if len(clean) > bestLen {
			best = cfg
			bestLen = len(clean)
		}
	}

	return best
}

func folderPathContains(watch, item string) bool {
	return folders.PathContains(watch, item)
}

func (u *Unpackerr) kindFromURL(url string) starr.App {
	if url == "" {
		return ""
	}

	for _, server := range u.Sonarr {
		if server != nil && server.URL == url {
			return starr.Sonarr
		}
	}

	for _, server := range u.Radarr {
		if server != nil && server.URL == url {
			return starr.Radarr
		}
	}

	for _, server := range u.Lidarr {
		if server != nil && server.URL == url {
			return starr.Lidarr
		}
	}

	for _, server := range u.Readarr {
		if server != nil && server.URL == url {
			return starr.Readarr
		}
	}

	return ""
}

func preFileKeys(files map[string]os.FileInfo) []string {
	if len(files) == 0 {
		return nil
	}

	keys := make([]string, 0, len(files))
	for path := range files {
		keys = append(keys, path)
	}

	slices.Sort(keys)

	return keys
}

func preFilesFromKeys(keys []string) map[string]os.FileInfo {
	if len(keys) == 0 {
		return nil
	}

	out := make(map[string]os.FileInfo, len(keys))
	for _, path := range keys {
		out[path] = nil
	}

	return out
}
