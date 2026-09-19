package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golift.io/cnfg"
)

func TestWaitExtensionsKeepsFolderWaiting(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	item := filepath.Join(watch, "Pending.Download.Test")

	if err := os.Mkdir(item, 0o700); err != nil {
		t.Fatal(err)
	}

	part := filepath.Join(item, "Pending.Download.Test.zip.part")
	if err := os.WriteFile(part, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: watch, WaitExtensions: []string{"part"}}
	unpack := newFolderUnpack(t, cfg)
	unpack.StartDelay.Duration = 0

	now := time.Now()
	unpack.processEvent(&eventData{Config: cfg, Name: "Pending.Download.Test", File: part, Op: "w CREATE"}, now)
	unpack.checkFolderStats(now)

	got := unpack.Map[item]
	if got == nil || got.Status != WAITING {
		t.Fatalf("queued despite wait file: %+v", got)
	}

	queue := unpack.queueFromExtract(item, got)
	if queue.Note != "Pending.Download.Test.zip.part" || queue.Progress != "last write" {
		t.Fatalf("queue %+v", queue)
	}

	if err := os.Remove(part); err != nil {
		t.Fatal(err)
	}

	unpack.StartDelay.Duration = time.Hour
	unpack.checkFolderStats(now.Add(cleanerInterval + time.Second))

	got = unpack.Map[item]
	if got == nil || got.Status != WAITING || got.Note != "" {
		t.Fatalf("after part removed %+v", got)
	}
}

func TestWaitExtensionsIgnoresNestedFiles(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	item := filepath.Join(watch, "Release")
	nested := filepath.Join(item, "nested")

	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(nested, "movie.zip.part"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(item, "movie.rar"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: watch, WaitExtensions: []string{".part"}}
	unpack := newFolderUnpack(t, cfg)
	unpack.StartDelay.Duration = time.Hour

	now := time.Now()
	unpack.processEvent(&eventData{Config: cfg, Name: "Release", File: item, Op: "w CREATE"}, now)
	unpack.checkFolderStats(now)

	got := unpack.Map[item]
	if got == nil || got.Note != "" {
		t.Fatalf("nested part should not wait: %+v", got)
	}
}

func TestSkipEmptyDropsArchiveFreeFolder(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	item := filepath.Join(watch, "Media.Only.Sample")

	if err := os.Mkdir(item, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(item, "video.mkv"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: watch, SkipEmpty: true}
	unpack := newFolderUnpack(t, cfg)
	unpack.StartDelay.Duration = 0
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	now := time.Now()
	unpack.processEvent(&eventData{Config: cfg, Name: "Media.Only.Sample", File: item, Op: "w CREATE"}, now)

	if unpack.Map[item] == nil {
		t.Fatal("expected waiting row during scan")
	}

	unpack.checkFolderStats(now)

	if unpack.Map[item] != nil {
		t.Fatal("empty folder stayed in the queue")
	}

	if _, ok := unpack.folders.Folders[item]; ok {
		t.Fatal("empty folder still tracked")
	}

	if got := unpack.historySnapshot(); len(got) != 0 {
		t.Fatalf("history %+v", got)
	}
}

func TestWaitScanSkippedWhileFSNotifyHot(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	item := filepath.Join(watch, "Hot.Writes")

	if err := os.Mkdir(item, 0o700); err != nil {
		t.Fatal(err)
	}

	part := filepath.Join(item, "Hot.Writes.rar.part")
	if err := os.WriteFile(part, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: watch, WaitExtensions: []string{".part"}}
	unpack := newFolderUnpack(t, cfg)
	unpack.StartDelay.Duration = 0

	now := time.Now()
	unpack.processEvent(&eventData{Config: cfg, Name: "Hot.Writes", File: part, Op: "f WRITE"}, now)

	if unpack.folders.Folders[item].Updated != now {
		t.Fatal("watch event must stamp Updated")
	}

	if err := os.Remove(part); err != nil {
		t.Fatal(err)
	}

	unpack.checkFolderStats(now)

	got := unpack.Map[item]
	if got == nil || got.Status != WAITING || got.Note != "Hot.Writes.rar.part" {
		t.Fatalf("hot fsnotify should skip wait ReadDir: %+v", got)
	}

	unpack.StartDelay.Duration = time.Hour
	unpack.checkFolderStats(now.Add(cleanerInterval + time.Second))

	got = unpack.Map[item]
	if got == nil || got.Status != WAITING || got.Note != "" {
		t.Fatalf("stale fsnotify should rescan: %+v", got)
	}
}

func TestQueueFromExtractWatchMode(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	item := filepath.Join(watch, "Show")

	if err := os.Mkdir(item, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: watch}
	unpack := newFolderUnpack(t, cfg)
	now := time.Now()
	unpack.processEvent(&eventData{Config: cfg, Name: "Show", File: item, Op: "f CREATE"}, now)

	got := unpack.queueFromExtract(item, unpack.Map[item])
	if got.Event != "fsnotify" {
		t.Fatalf("event %q", got.Event)
	}

	pollWatch := t.TempDir()
	pollItem := filepath.Join(pollWatch, "Show")

	if err := os.Mkdir(pollItem, 0o700); err != nil {
		t.Fatal(err)
	}

	pollCfg := &FolderConfig{Path: pollWatch, Interval: cnfg.Duration{Duration: time.Second}}
	pollUnpack := newFolderUnpack(t, pollCfg)
	pollUnpack.processEvent(&eventData{Config: pollCfg, Name: "Show", File: pollItem, Op: "w CREATE"}, now)

	got = pollUnpack.queueFromExtract(pollItem, pollUnpack.Map[pollItem])
	if got.Event != "polling" {
		t.Fatalf("event %q", got.Event)
	}
}

func newFolderUnpack(t *testing.T, cfg *FolderConfig) *Unpackerr {
	t.Helper()

	unpack := New()
	unpack.Folder.Buffer = 32

	tracker, err := unpack.Folder.NewWatcher([]*FolderConfig{cfg}, unpack.Logger, updateChanBuf, suffix)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(tracker.Close)

	unpack.folders = tracker

	return unpack
}
