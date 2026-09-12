package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFolderWaitingShowsInQueue(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	cfg := &FolderConfig{Path: watch}
	unpack := New()

	unpack.Folder.Buffer = 32

	tracker, err := unpack.Folder.NewWatcher([]*FolderConfig{cfg}, unpack.Logger, updateChanBuf, suffix)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if tracker.Watcher != nil {
			tracker.Watcher.Close()
		}

		if tracker.FSNotify != nil {
			_ = tracker.FSNotify.Close()
		}
	})

	unpack.folders = tracker

	archive := filepath.Join(watch, "movie.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	first := time.Now().Add(-time.Minute)
	unpack.processEvent(&eventData{Config: cfg, Name: "movie.rar", File: archive, Op: "test"}, first)

	item := unpack.Map[archive]
	if item == nil || item.Status != WAITING || item.App != FolderString {
		t.Fatalf("queue item %+v", item)
	}

	if got := queueFromExtract(archive, item); got.Progress != "last write" {
		t.Fatalf("progress %q", got.Progress)
	}

	later := first.Add(30 * time.Second)
	unpack.processEvent(&eventData{Config: cfg, Name: "movie.rar", File: archive, Op: "write"}, later)

	if !unpack.Map[archive].Updated.Equal(unpack.folders.Folders[archive].Updated) {
		t.Fatalf("last write %v folder %v", unpack.Map[archive].Updated, unpack.folders.Folders[archive].Updated)
	}

	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}

	unpack.processEvent(&eventData{Config: cfg, Name: "movie.rar", File: archive, Op: "remove"}, later.Add(time.Second))

	if unpack.Map[archive] != nil {
		t.Fatal("waiting item still in queue after delete")
	}
}

func TestCheckFolderStatsDropsMissingWaiting(t *testing.T) {
	t.Parallel()

	watch := t.TempDir()
	cfg := &FolderConfig{Path: watch}
	unpack := New()
	unpack.Folder.Buffer = 32

	tracker, err := unpack.Folder.NewWatcher([]*FolderConfig{cfg}, unpack.Logger, updateChanBuf, suffix)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if tracker.Watcher != nil {
			tracker.Watcher.Close()
		}

		if tracker.FSNotify != nil {
			_ = tracker.FSNotify.Close()
		}
	})

	unpack.folders = tracker

	archive := filepath.Join(watch, "movie.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack.processEvent(&eventData{Config: cfg, Name: "movie.rar", File: archive, Op: "test"}, time.Now())

	if unpack.Map[archive] == nil {
		t.Fatal("expected waiting queue item")
	}

	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}

	unpack.checkFolderStats(time.Now())

	if unpack.Map[archive] != nil {
		t.Fatal("waiting item still in queue after checkFolderStats")
	}

	if _, ok := unpack.folders.Folders[archive]; ok {
		t.Fatal("folder still tracked after checkFolderStats")
	}
}
