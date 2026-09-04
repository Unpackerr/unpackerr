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
	"time"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
)

const (
	historyFileName = "unpackerr.history.jsonl"
	historyScanMax  = 1024 * 1024
)

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

func (status ExtractStatus) isDurableHistory() bool {
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
		u.Printf("[Unpackerr] History file disabled; keep_history=%d but no log, config, or home path", u.KeepHistory)

		return
	}

	records := u.readHistoryRecords()
	trimmed := false

	if limit := int(u.KeepHistory); limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
		trimmed = true
	}

	u.histMu.Lock()
	u.records = records

	if trimmed {
		if err := u.writeHistoryLocked(); err != nil {
			u.Errorf("Writing history file: %v", err)
		}
	}

	u.histMu.Unlock()
}

func (u *Unpackerr) readHistoryRecords() []HistoryRecord {
	file, err := os.Open(u.histPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			u.Errorf("Opening history file: %v", err)
		}

		return nil
	}

	defer file.Close()

	reader := bufio.NewReader(file)

	var records []HistoryRecord

	for {
		line, readErr := reader.ReadBytes('\n')
		records = u.appendHistoryLine(records, line)

		if errors.Is(readErr, io.EOF) {
			break
		}

		if readErr != nil {
			u.Errorf("Reading history file: %v", readErr)

			break
		}
	}

	return records
}

func (u *Unpackerr) appendHistoryLine(records []HistoryRecord, line []byte) []HistoryRecord {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return records
	}

	if len(line) > historyScanMax {
		u.Errorf("Skipping oversized history line")

		return records
	}

	var rec HistoryRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		u.Errorf("Skipping bad history line: %v", err)

		return records
	}

	return append(records, rec)
}

func (u *Unpackerr) maybeRecordHistory(itemID string, item *Extract) {
	if item == nil || u.KeepHistory == 0 || !item.Status.isDurableHistory() {
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

func (u *Unpackerr) upsertHistory(rec HistoryRecord) {
	if rec.ID == "" {
		rec.ID = rec.Path
	}

	u.histMu.Lock()
	defer u.histMu.Unlock()

	found := -1

	for idx := range u.records {
		if u.records[idx].ID == rec.ID {
			found = idx
			if rec.Started.IsZero() {
				rec.Started = u.records[idx].Started
			}

			break
		}
	}

	if found >= 0 {
		u.records = append(u.records[:found], u.records[found+1:]...)
	}

	u.records = append(u.records, rec)

	if limit := int(u.KeepHistory); limit > 0 && len(u.records) > limit {
		u.records = u.records[len(u.records)-limit:]
	}

	if err := u.writeHistoryLocked(); err != nil {
		u.Errorf("Writing history file: %v", err)
	}
}

func (u *Unpackerr) writeHistoryLocked() error {
	if u.histPath == "" {
		return nil
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)

	for idx := range u.records {
		if err := enc.Encode(u.records[idx]); err != nil {
			return fmt.Errorf("encoding history: %w", err)
		}
	}

	if err := configdef.AtomicWrite(u.histPath, buf.Bytes()); err != nil {
		return fmt.Errorf("writing history file: %w", err)
	}

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
