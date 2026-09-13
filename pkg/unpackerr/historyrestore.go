package unpackerr

import (
	"errors"
	"os"
	"slices"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

// restoreQueueFromHistory copies recent Starr rows from the JSONL into History.Map
// so a crash or kill does not lose EXTRACTED-awaiting-import or IMPORTED-awaiting-delete.
// Folder rows stay out: the watch tracker rebuilds those. Call after validateApps so
// URL→dialect matching sees the live Starr list. Does not take histMu and History.mu
// at the same time (updateQueueStatus holds History.mu then histMu).
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

	for i := range rows {
		item, itemID, ok := u.extractFromHistory(rows[i], now, cutoff)
		if !ok {
			continue
		}

		if _, exists := u.Map[itemID]; exists {
			continue
		}

		u.Map[itemID] = item
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
	if kind == FolderString || (restoreNeedsKind(status) && kind == "") {
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

	if len(rec.NewFiles) == 0 && rec.Bytes == 0 && errMsg == "" {
		return
	}

	item.Resp = &xtractr.Response{
		NewFiles: append([]string(nil), rec.NewFiles...),
		Size:     rec.Bytes,
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
	case EXTRACTED, IMPORTED, EXTRACTFAILED:
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
	case EXTRACTED, WAITING, EXTRACTFAILED:
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
