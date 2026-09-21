package unpackerr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	historyFileName = "unpackerr.history.jsonl"
	historyFileMode = 0o600
	// historyCompactFactor: rewrite the file once appended lines exceed this
	// many times keep_history, so the on-disk log stays bounded.
	historyCompactFactor = 2
	// historyRestoreAge is how long a JSONL row may sit and still be copied
	// back into the live queue after a restart.
	historyRestoreAge = 72 * time.Hour
)

var (
	errHistoryNotFound    = errors.New("not found")
	errHistoryInFlight    = errors.New("item is still in progress")
	errInterruptedRestart = errors.New("interrupted by restart")
)

// HistoryRecord is one JSONL row (history API + restart resume).
type HistoryRecord struct {
	ID           string            `json:"id"`
	App          string            `json:"app"`
	Kind         string            `json:"kind,omitempty"` // Starr dialect or Folder; App is the instance label.
	URL          string            `json:"url,omitempty"`
	Path         string            `json:"path"`
	OutputPath   string            `json:"outputPath,omitempty"`
	Status       ExtractStatus     `json:"status"`
	Retries      uint              `json:"retries"`
	HookFail     uint              `json:"hookFail,omitempty"`
	HookMessages map[string]string `json:"hookMessages,omitempty"`
	Started      time.Time         `json:"started"`
	Updated      time.Time         `json:"updated"`
	Finished     time.Time         `json:"finished,omitzero"`
	Archives     int               `json:"archives,omitempty"`
	Files        int               `json:"files,omitempty"`
	Bytes        uint64            `json:"bytes,omitempty"`
	Ratio        float64           `json:"ratio,omitempty"`
	Elapsed      string            `json:"elapsed,omitempty"`
	Error        string            `json:"error,omitempty"`
	Progress     string            `json:"progress,omitempty"`
	DeleteOrig   bool              `json:"deleteOrig,omitempty"`
	DeleteDelay  string            `json:"deleteDelay,omitempty"` // Go duration, e.g. 5m0s
	Syncthing    bool              `json:"syncthing,omitempty"`
	SplitFlac    bool              `json:"splitFlac,omitempty"`
	MaxBytes     uint64            `json:"maxBytes,omitempty"`
	NoRetry      bool              `json:"noRetry,omitempty"`
	NewFiles     []string          `json:"newFiles,omitempty"`
	OrigFiles    []string          `json:"origFiles,omitempty"`  // Archive paths; folder delete_orig after a restart.
	ExtraFiles   []string          `json:"extraFiles,omitempty"` // Nested extras; display + restore Resp.Extras.
	PreFiles     []string          `json:"preFiles,omitempty"`
	Forgotten    bool              `json:"forgotten,omitempty"`
	IDs          map[string]any    `json:"ids,omitempty"` // Starr hook metadata (title, downloadId, …).
	Event        string            `json:"event,omitempty"`
	Queue        int               `json:"queue,omitempty"`  // xtractr waiting count when this extract started.
	Output       string            `json:"output,omitempty"` // xtractr dest folder.
}

// QueueItem is a live in-flight extract for GET /api/queue.
type QueueItem struct {
	ID          string         `json:"id"`
	App         string         `json:"app"`
	URL         string         `json:"url,omitempty"`
	Path        string         `json:"path"`
	OutputPath  string         `json:"outputPath,omitempty"`
	Status      ExtractStatus  `json:"status"`
	Retries     uint           `json:"retries"`
	HookFail    uint           `json:"hookFail,omitempty"`
	Updated     time.Time      `json:"updated"`
	Progress    string         `json:"progress,omitempty"`
	Error       string         `json:"error,omitempty"`
	Percent     float64        `json:"percent,omitempty"`
	Wrote       uint64         `json:"wrote,omitempty"`
	Total       uint64         `json:"total,omitempty"`
	Read        uint64         `json:"read,omitempty"`
	Compressed  uint64         `json:"compressed,omitempty"`
	Files       int            `json:"files,omitempty"`
	Count       int            `json:"count,omitempty"`
	Archives    int            `json:"archives,omitempty"`
	Extracted   int            `json:"extracted,omitempty"`
	Archive     string         `json:"archive,omitempty"`
	SpeedBps    uint64         `json:"speedBps,omitempty"`    // last sample interval
	AvgSpeedBps uint64         `json:"avgSpeedBps,omitempty"` // bytes so far / extract duration
	ETA         time.Time      `json:"eta,omitzero"`
	Due         time.Time      `json:"due,omitzero"`
	DueKind     string         `json:"dueKind,omitempty"` // start, retry, cleanup, history
	Note        string         `json:"note,omitempty"`
	Event       string         `json:"event,omitempty"` // folders: fsnotify, polling; later manual
	Started     time.Time      `json:"started,omitzero"`
	Elapsed     string         `json:"elapsed,omitempty"`
	Bytes       uint64         `json:"bytes,omitempty"`
	Ratio       float64        `json:"ratio,omitempty"`
	Queue       int            `json:"queue,omitempty"`  // xtractr waiting count when this extract started.
	Output      string         `json:"output,omitempty"` // xtractr dest folder.
	Kind        string         `json:"kind,omitempty"`
	IDs         map[string]any `json:"ids,omitempty"`
	NewFiles    []string       `json:"newFiles,omitempty"`  // GET /api/queue/item only; omitted from progress.
	OrigFiles   []string       `json:"origFiles,omitempty"` // GET /api/queue/item only; omitted from progress.
}

func isDurableHistory(status ExtractStatus) bool {
	switch status {
	case EXTRACTFAILED, EXTRACTEDNOTHING, IMPORTED, DELETED, DELETEFAILED:
		return true
	default:
		return false
	}
}

// isPersistedHistory is written to JSONL so a restart can rebuild the live queue.
// Starr WAITING is left out; the next poll recreates it. Folder WAITING is kept so
// a retry after EXTRACTFAILED cannot restore the old failed row.
func isPersistedHistory(item *Extract) bool {
	if item == nil {
		return false
	}

	if item.Status == WAITING && item.App == FolderString {
		return true
	}

	switch item.Status {
	case QUEUED, EXTRACTING, EXTRACTFAILED, EXTRACTED, IMPORTED,
		DELETING, DELETEFAILED, DELETED, EXTRACTEDNOTHING:
		return true
	default:
		return false
	}
}

func (u *Unpackerr) historyFilePath() string {
	if u.LogFile != "" && u.rotatorr != nil {
		return filepath.Join(filepath.Dir(u.LogFile), historyFileName)
	}

	if u.ConfigFile != "" {
		return filepath.Join(filepath.Dir(u.ConfigFile), historyFileName)
	}

	// Env-only (stdout logs, no config file): keep history in memory only.
	return ""
}

func (u *Unpackerr) loadHistory() {
	if u.KeepHistory == 0 {
		return
	}

	if u.histPath == "" {
		u.histPath = u.historyFilePath()
	}

	if u.histPath == "" {
		u.Printf("[Unpackerr] History file disabled; keep_history=%d but no log or config file",
			u.KeepHistory)

		return
	}

	lines, records := u.readHistoryRecords()

	u.histMu.Lock()
	defer u.histMu.Unlock()

	u.records = u.capHistoryLocked(mergeHistory(nil, records...))
	u.histLines = lines

	// The file is append-only; fold duplicates and over-cap rows on startup.
	if lines != len(u.records) {
		if err := u.compactHistoryLocked(); err != nil {
			u.Errorf("Compacting history file: %v", err)
		}
	}
}

// readHistoryRecords returns the line count and every parseable record, in file
// order. This is a file we write ourselves; bad lines are skipped, not fatal.
func (u *Unpackerr) readHistoryRecords() (int, []HistoryRecord) {
	file, err := os.Open(u.histPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			u.Errorf("Opening history file: %v", err)
		}

		return 0, nil
	}
	defer file.Close()

	var (
		reader  = bufio.NewReader(file)
		records []HistoryRecord
		lines   int
	)

	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			lines++

			var rec HistoryRecord
			if jsonErr := json.Unmarshal(line, &rec); jsonErr != nil {
				u.Errorf("Skipping bad history line: %v", jsonErr)
			} else {
				records = append(records, rec)
			}
		}

		if errors.Is(err, io.EOF) {
			return lines, records
		} else if err != nil {
			u.Errorf("Reading history file: %v", err)
			return lines, records
		}
	}
}

// mergeHistory upserts recs onto list by ID, keeping the earliest Started.
func mergeHistory(list []HistoryRecord, recs ...HistoryRecord) []HistoryRecord {
	for _, rec := range recs {
		if rec.ID == "" {
			rec.ID = rec.Path
		}

		if idx := slices.IndexFunc(list, func(r HistoryRecord) bool { return r.ID == rec.ID }); idx >= 0 {
			if rec.Started.IsZero() {
				rec.Started = list[idx].Started
			}

			list = slices.Delete(list, idx, idx+1)
		}

		list = append(list, rec)
	}

	return list
}

// capHistoryLocked trims completed/failed rows to keep_history. In-flight
// checkpoints sit on top of that cap until they finish, so a small
// keep_history cannot drop EXTRACTED work a restart would resume.
func (u *Unpackerr) capHistoryLocked(list []HistoryRecord) []HistoryRecord {
	limit := int(u.KeepHistory)
	if limit <= 0 {
		return list
	}

	durable := 0

	for _, rec := range list {
		if isDurableHistory(rec.Status) {
			durable++
		}
	}

	if durable <= limit {
		return list
	}

	drop := durable - limit
	out := make([]HistoryRecord, 0, len(list)-drop)

	for _, rec := range list {
		if drop > 0 && isDurableHistory(rec.Status) {
			drop--
			continue
		}

		out = append(out, rec)
	}

	return out
}

func (u *Unpackerr) maybeRecordHistory(itemID string, item *Extract) {
	if u.KeepHistory == 0 || !isPersistedHistory(item) {
		return
	}

	u.upsertHistory(historyFromExtract(itemID, item))
}

func historyFromExtract(itemID string, item *Extract) HistoryRecord {
	now := item.Updated
	if itemID == "" {
		itemID = item.Path
	}

	rec := HistoryRecord{
		ID:           itemID,
		App:          item.Label(),
		Kind:         string(item.App),
		URL:          item.URL,
		Path:         item.Path,
		OutputPath:   item.OutputPath,
		Status:       item.Status,
		Retries:      item.Retries,
		HookFail:     item.HookFail,
		HookMessages: maps.Clone(item.HookMessages),
		Started:      now,
		Updated:      now,
		DeleteOrig:   item.DeleteOrig,
		Syncthing:    item.Syncthing,
		SplitFlac:    item.SplitFlac,
		MaxBytes:     item.MaxBytes,
		NoRetry:      item.NoRetry,
		PreFiles:     preFileKeys(item.PreFiles),
		IDs:          cloneIDs(item.IDs),
		Event:        item.Event,
	}

	if item.DeleteDelay != 0 {
		rec.DeleteDelay = item.DeleteDelay.String()
	}

	if isDurableHistory(item.Status) {
		rec.Finished = now
	}

	fillHistoryStats(&rec, item)

	return rec
}

func fillHistoryStats(rec *HistoryRecord, item *Extract) {
	if item.XProg != nil {
		if prog := item.XProg.String(); prog != "no progress yet" {
			rec.Progress = prog
		}

		if item.XProg.Progress != nil && item.XProg.Compressed > 0 && item.Resp != nil && item.Resp.Size > 0 {
			rec.Ratio = float64(item.Resp.Size) / float64(item.XProg.Compressed)
		}
	}

	if item.Resp == nil {
		return
	}

	if !item.Resp.Started.IsZero() {
		rec.Started = item.Resp.Started
	}

	rec.Archives = item.Resp.Archives.Count() + item.Resp.Extras.Count()
	rec.Files = len(item.Resp.NewFiles)
	rec.Bytes = item.Resp.Size
	rec.Queue = item.Resp.Queued
	rec.Output = item.Resp.Output
	rec.NewFiles = slices.Clone(item.Resp.NewFiles)
	rec.OrigFiles = slices.Clone(item.Resp.Archives.List())
	rec.ExtraFiles = slices.Clone(item.Resp.Extras.List())

	if item.Resp.Elapsed > 0 {
		rec.Elapsed = item.Resp.Elapsed.Round(time.Second).String()
	}

	if item.Resp.Error != nil {
		rec.Error = item.Resp.Error.Error()
	}
}

// upsertHistory records one persisted transition: update memory, append one
// line. The file is compacted only when appends outgrow the cap.
func (u *Unpackerr) upsertHistory(rec HistoryRecord) {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	u.records = u.capHistoryLocked(mergeHistory(u.records, rec))
	if len(u.records) == 0 {
		return
	}

	saved := u.records[len(u.records)-1]
	u.notifyHistoryLocked(saved)

	if u.histPath == "" {
		return
	}

	if limit := int(u.KeepHistory); limit > 0 && u.histLines >= limit*historyCompactFactor {
		if err := u.compactHistoryLocked(); err != nil {
			u.Errorf("Compacting history file: %v", err)
		}

		return
	}

	line, err := json.Marshal(u.records[len(u.records)-1])
	if err != nil {
		u.Errorf("Encoding history: %v", err)
		return
	}

	file, err := os.OpenFile(u.histPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, historyFileMode)
	if err != nil {
		u.Errorf("Opening history file: %v", err)
		return
	}
	defer file.Close()

	if _, err := file.Write(append(line, '\n')); err != nil {
		u.Errorf("Writing history file: %v", err)
		return
	}

	u.histLines++
}

// notifyHistoryLocked pushes UI history. In-flight JSONL rows (extracting,
// extracted, and similar) stay on disk for restart resume but are not history rows.
func (u *Unpackerr) notifyHistoryLocked(rec HistoryRecord) {
	if u.hub == nil {
		return
	}

	if isDurableHistory(rec.Status) {
		row := rec
		u.hub.notify(topicHistory, historyFrame{Op: "upsert", Row: &row})

		return
	}

	if rec.ID != "" {
		u.hub.notify(topicHistory, historyFrame{Op: "delete", ID: rec.ID})
	}
}

// compactHistoryLocked rewrites the file from memory: one line per record.
func (u *Unpackerr) compactHistoryLocked() error {
	if u.histPath == "" {
		return nil
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	for idx := range u.records {
		_ = enc.Encode(u.records[idx]) //nolint:errchkjson // ExtractStatus has a MarshalText that cannot fail.
	}

	if err := os.MkdirAll(filepath.Dir(u.histPath), logsDirMode); err != nil {
		return fmt.Errorf("making history dir: %w", err)
	}

	if err := os.WriteFile(u.histPath, buf.Bytes(), historyFileMode); err != nil {
		return fmt.Errorf("writing history file: %w", err)
	}

	u.histLines = len(u.records)

	return nil
}

func (u *Unpackerr) historySnapshot() []HistoryRecord {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	out := make([]HistoryRecord, 0, len(u.records))

	for _, rec := range slices.Backward(u.records) {
		if isDurableHistory(rec.Status) {
			out = append(out, rec)
		}
	}

	return out
}

func (u *Unpackerr) queueSnapshot() []QueueItem {
	u.rLockHistory()
	defer u.rUnlockHistory()

	return u.queueSnapshotLocked()
}

func (u *Unpackerr) queueFromExtract(itemID string, item *Extract) QueueItem {
	queue := QueueItem{
		ID:         itemID,
		App:        item.Label(),
		URL:        item.URL,
		Path:       item.Path,
		OutputPath: item.OutputPath,
		Status:     item.Status,
		Retries:    item.Retries,
		HookFail:   item.HookFail,
		Updated:    item.Updated,
		Due:        item.Due,
		DueKind:    item.DueKind,
		Note:       item.Note,
		Event:      item.Event,
		Kind:       string(item.App),
		IDs:        cloneIDs(item.IDs),
	}

	if item.Status == WAITING && item.App == FolderString {
		queue.Progress = "last write"
	}

	if item.Note != "" && queue.Progress == "" {
		queue.Progress = item.Note
	}

	fillQueueProgress(&queue, item)
	fillQueueMeta(&queue, item)

	if item.Resp != nil && item.Resp.Error != nil {
		queue.Error = item.Resp.Error.Error()
	}

	return queue
}

func fillQueueMeta(queue *QueueItem, item *Extract) {
	if item.XProg != nil && item.XProg.Progress != nil && item.XProg.Compressed > 0 &&
		item.Resp != nil && item.Resp.Size > 0 {
		queue.Ratio = float64(item.Resp.Size) / float64(item.XProg.Compressed)
	}

	if item.Resp == nil {
		return
	}

	if !item.Resp.Started.IsZero() {
		queue.Started = item.Resp.Started
		switch {
		case item.Resp.Elapsed > 0:
			queue.Elapsed = item.Resp.Elapsed.Round(time.Second).String()
		case item.Status == EXTRACTING:
			queue.Elapsed = time.Since(item.Resp.Started).Round(time.Second).String()
		}
	}

	queue.Bytes = item.Resp.Size
	queue.Queue = item.Resp.Queued
	queue.Output = item.Resp.Output
}

// fillQueueFiles copies archive and extracted paths. Progress websocket frames
// omit them so an ISO extract does not push thousands of paths every tick.
func fillQueueFiles(queue *QueueItem, item *Extract) {
	if item.Resp == nil {
		return
	}

	if n := len(item.Resp.NewFiles); n > 0 {
		queue.NewFiles = slices.Clone(item.Resp.NewFiles)
	}

	if archives := respArchivePaths(item); len(archives) > 0 {
		queue.OrigFiles = archives
	}
}

func respArchivePaths(item *Extract) []string {
	if item == nil || item.Resp == nil {
		return nil
	}

	return append(slices.Clone(item.Resp.Archives.List()), item.Resp.Extras.List()...)
}

func cloneIDs(ids map[string]any) map[string]any {
	if len(ids) == 0 {
		return nil
	}

	out := make(map[string]any, len(ids))
	maps.Copy(out, ids)

	return out
}

func fillQueueProgress(queue *QueueItem, item *Extract) {
	if item.XProg == nil {
		return
	}

	if prog := item.XProg.String(); prog != "no progress yet" {
		queue.Progress = prog
	}

	prog := item.XProg.Progress
	if prog == nil {
		return
	}

	queue.Percent = prog.Percent()
	queue.Wrote = prog.Wrote
	queue.Total = prog.Total
	queue.Read = prog.Read
	queue.Compressed = prog.Compressed
	queue.Files = prog.Files
	queue.Count = prog.Count
	queue.Archives = item.XProg.Archives
	queue.Extracted = item.XProg.Extracted
	queue.SpeedBps = item.XProg.SpeedBps
	queue.AvgSpeedBps = item.XProg.AvgSpeedBps
	queue.ETA = item.XProg.ETA

	if prog.XFile != nil {
		rel := strings.TrimPrefix(prog.XFile.FilePath, item.Path)
		queue.Archive = strings.TrimLeft(filepath.ToSlash(rel), `/\`)
	}
}

const (
	dueStart   = "start"
	dueRetry   = "retry"
	dueCleanup = "cleanup"
	dueHistory = "history"
)

// stampQueueDue writes the next timer onto the extract. Call from the main
// loop after Status or Updated changes; HTTP readers only copy the fields.
func (u *Unpackerr) stampQueueDue(itemID string, item *Extract) {
	if item == nil {
		return
	}

	item.Due, item.DueKind = u.queueDue(itemID, item)
}

func (u *Unpackerr) queueDue(itemID string, item *Extract) (time.Time, string) {
	switch item.Status {
	case WAITING:
		if u.StartDelay.Duration <= 0 {
			return time.Time{}, ""
		}

		if item.App != FolderString && item.Note != "" {
			return time.Time{}, ""
		}

		return item.Updated.Add(u.StartDelay.Duration), dueStart
	case EXTRACTFAILED:
		if item.NoRetry || item.Retries >= u.maxRetries() {
			return time.Time{}, ""
		}

		return item.Updated.Add(u.RetryDelay.Duration), dueRetry
	case EXTRACTED:
		delay := u.folderDeleteAfter(itemID, item)
		if delay <= 0 {
			return time.Time{}, ""
		}

		return item.Updated.Add(delay), dueCleanup
	case IMPORTED:
		if item.DeleteDelay < 0 {
			return time.Time{}, ""
		}

		return item.Updated.Add(item.DeleteDelay), dueCleanup
	case DELETED:
		return item.Updated.Add(item.DeleteDelay), dueHistory
	case EXTRACTEDNOTHING:
		if item.App != FolderString || u.StartDelay.Duration <= 0 {
			return time.Time{}, ""
		}

		return item.Updated.Add(u.StartDelay.Duration), dueHistory
	default:
		return time.Time{}, ""
	}
}

func (u *Unpackerr) folderDeleteAfter(itemID string, item *Extract) time.Duration {
	if item.App != FolderString {
		return 0
	}

	if u.folders != nil {
		if folder := u.folders.Folders[itemID]; folder != nil && folder.Config != nil && folder.Config.DeleteAfter != nil {
			return folder.Config.DeleteAfter.Duration
		}
	}

	return item.DeleteDelay
}

func (u *Unpackerr) deleteHistoryID(itemID string) error {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	idx := slices.IndexFunc(u.records, func(r HistoryRecord) bool { return r.ID == itemID })
	if idx < 0 {
		return errHistoryNotFound
	}

	if !isDurableHistory(u.records[idx].Status) {
		return errHistoryInFlight
	}

	u.records = slices.Delete(u.records, idx, idx+1)

	if u.hub != nil {
		u.hub.notify(topicHistory, historyFrame{Op: "delete", ID: itemID})
	}

	return u.compactHistoryLocked()
}

// markHistoryForgotten keeps the history row and flags it so a restart does
// not restore the item into Map (and re-arm post-import delete).
func (u *Unpackerr) markHistoryForgotten(itemID string) {
	if u.KeepHistory == 0 || itemID == "" {
		return
	}

	u.histMu.Lock()

	idx := slices.IndexFunc(u.records, func(r HistoryRecord) bool { return r.ID == itemID })
	if idx < 0 {
		u.histMu.Unlock()
		return
	}

	rec := u.records[idx]
	u.histMu.Unlock()

	rec.Forgotten = true
	u.upsertHistory(rec)
}

func (u *Unpackerr) clearHistory() error {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	u.records = slices.DeleteFunc(u.records, func(rec HistoryRecord) bool {
		return isDurableHistory(rec.Status)
	})

	if u.hub != nil {
		u.hub.notify(topicHistory, historyFrame{Op: "clear"})
	}

	return u.compactHistoryLocked()
}
