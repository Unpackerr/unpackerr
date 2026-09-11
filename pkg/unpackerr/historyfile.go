package unpackerr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	historyFileName = "unpackerr.history.jsonl"
	historyFileMode = 0o600
	// historyCompactFactor: rewrite the file once appended lines exceed this
	// many times keep_history, so the on-disk log stays bounded.
	historyCompactFactor = 2
)

var errHistoryNotFound = errors.New("not found")

// HistoryRecord is one completed or failed pipeline item (JSONL + API).
type HistoryRecord struct {
	ID         string        `json:"id"`
	App        string        `json:"app"`
	URL        string        `json:"url,omitempty"`
	Path       string        `json:"path"`
	OutputPath string        `json:"outputPath,omitempty"`
	Status     ExtractStatus `json:"status"`
	Retries    uint          `json:"retries"`
	Started    time.Time     `json:"started"`
	Updated    time.Time     `json:"updated"`
	Finished   time.Time     `json:"finished"`
	Archives   int           `json:"archives,omitempty"`
	Files      int           `json:"files,omitempty"`
	Bytes      uint64        `json:"bytes,omitempty"`
	Ratio      float64       `json:"ratio,omitempty"`
	Elapsed    string        `json:"elapsed,omitempty"`
	Error      string        `json:"error,omitempty"`
	Progress   string        `json:"progress,omitempty"`
}

// QueueItem is a live in-flight extract for GET /api/queue.
type QueueItem struct {
	ID         string        `json:"id"`
	App        string        `json:"app"`
	URL        string        `json:"url,omitempty"`
	Path       string        `json:"path"`
	OutputPath string        `json:"outputPath,omitempty"`
	Status     ExtractStatus `json:"status"`
	Retries    uint          `json:"retries"`
	Updated    time.Time     `json:"updated"`
	Progress   string        `json:"progress,omitempty"`
	Error      string        `json:"error,omitempty"`
}

func isDurableHistory(status ExtractStatus) bool {
	switch status {
	case EXTRACTFAILED, EXTRACTEDNOTHING, IMPORTED, DELETED, DELETEFAILED:
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

	return expandHomedir(filepath.Join("~", ".unpackerr", historyFileName))
}

func (u *Unpackerr) loadHistory() {
	if u.KeepHistory == 0 {
		return
	}

	if u.histPath == "" {
		u.histPath = u.historyFilePath()
	}

	if u.histPath == "" {
		u.Printf("[Unpackerr] History file disabled; keep_history=%d but no log, config, or home path",
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

func (u *Unpackerr) capHistoryLocked(list []HistoryRecord) []HistoryRecord {
	if limit := int(u.KeepHistory); limit > 0 && len(list) > limit {
		return list[len(list)-limit:]
	}

	return list
}

func (u *Unpackerr) maybeRecordHistory(itemID string, item *Extract) {
	if u.KeepHistory == 0 || !isDurableHistory(item.Status) {
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
		ID:         itemID,
		App:        string(item.App),
		URL:        item.URL,
		Path:       item.Path,
		OutputPath: item.OutputPath,
		Status:     item.Status,
		Retries:    item.Retries,
		Started:    now,
		Updated:    now,
		Finished:   now,
	}

	if item.XProg != nil {
		if prog := item.XProg.String(); prog != "no progress yet" {
			rec.Progress = prog
		}

		if item.XProg.Progress != nil && item.XProg.Compressed > 0 && item.Resp != nil && item.Resp.Size > 0 {
			rec.Ratio = float64(item.Resp.Size) / float64(item.XProg.Compressed)
		}
	}

	if item.Resp != nil {
		if !item.Resp.Started.IsZero() {
			rec.Started = item.Resp.Started
		}

		rec.Archives = item.Resp.Archives.Count() + item.Resp.Extras.Count()
		rec.Files = len(item.Resp.NewFiles)
		rec.Bytes = item.Resp.Size

		if item.Resp.Elapsed > 0 {
			rec.Elapsed = item.Resp.Elapsed.Round(time.Second).String()
		}

		if item.Resp.Error != nil {
			rec.Error = item.Resp.Error.Error()
		}
	}

	return rec
}

// upsertHistory records one durable transition: update memory, append one
// line. The file is compacted only when appends outgrow the cap.
func (u *Unpackerr) upsertHistory(rec HistoryRecord) {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	u.records = u.capHistoryLocked(mergeHistory(u.records, rec))

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

	out := make([]HistoryRecord, len(u.records))
	for idx := range u.records {
		out[len(out)-1-idx] = u.records[idx]
	}

	return out
}

func (u *Unpackerr) queueSnapshot() []QueueItem {
	u.rLockHistory()
	defer u.rUnlockHistory()

	out := make([]QueueItem, 0, len(u.Map))

	for name, item := range u.Map {
		out = append(out, queueFromExtract(name, item))
	}

	return out
}

func queueFromExtract(id string, item *Extract) QueueItem {
	queue := QueueItem{
		ID:         id,
		App:        string(item.App),
		URL:        item.URL,
		Path:       item.Path,
		OutputPath: item.OutputPath,
		Status:     item.Status,
		Retries:    item.Retries,
		Updated:    item.Updated,
	}

	if item.XProg != nil {
		if prog := item.XProg.String(); prog != "no progress yet" {
			queue.Progress = prog
		}
	}

	if item.Resp != nil && item.Resp.Error != nil {
		queue.Error = item.Resp.Error.Error()
	}

	return queue
}

func (u *Unpackerr) deleteHistoryID(itemID string) error {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	idx := slices.IndexFunc(u.records, func(r HistoryRecord) bool { return r.ID == itemID })
	if idx < 0 {
		return errHistoryNotFound
	}

	u.records = slices.Delete(u.records, idx, idx+1)

	return u.compactHistoryLocked()
}

func (u *Unpackerr) clearHistory() error {
	u.histMu.Lock()
	defer u.histMu.Unlock()

	u.records = nil

	return u.compactHistoryLocked()
}
