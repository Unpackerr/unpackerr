package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/starr/sonarr"
	"golift.io/xtractr"
)

func writeSized(t *testing.T, path string, size int) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), defaultDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, make([]byte, size), defaultFileMode); err != nil {
		t.Fatal(err)
	}
}

func TestSyncthingProgress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "a.rar"), 100)
	writeSized(t, filepath.Join(dir, "Sub", "a.r00"), 100) // subfolders count too.

	temp := filepath.Join(dir, ".syncthing.a.r01.tmp")
	writeSized(t, temp, 100) // a temp file may already have its final size; it must not count.

	start := time.Now()
	item := &Extract{Path: dir, Size: 300}

	if have, state := syncthingProgress(item, start); state != syncWaiting || have != 200 {
		t.Fatalf("200 of 300 bytes: have %d state %d, want 200 waiting", have, state)
	}

	// A write to the temp file is progress: the stall clock starts again.
	later := start.Add(syncthingStall - time.Minute)
	if err := os.Chtimes(temp, later, later); err != nil {
		t.Fatal(err)
	}

	if _, state := syncthingProgress(item, later); state != syncWaiting {
		t.Fatalf("temp file written: state %d, want waiting", state)
	}

	if _, state := syncthingProgress(item, later.Add(syncthingStall-time.Second)); state != syncWaiting {
		t.Fatalf("inside the stall window after progress: state %d, want waiting", state)
	}

	if _, state := syncthingProgress(item, later.Add(syncthingStall)); state != syncStalled {
		t.Fatalf("no change for the stall window: state %d, want stalled", state)
	}

	// Syncthing renames the finished temp file into place.
	if err := os.Rename(temp, filepath.Join(dir, "a.r01")); err != nil {
		t.Fatal(err)
	}

	if have, state := syncthingProgress(item, later.Add(2*syncthingStall)); state != syncDone || have != 300 {
		t.Fatalf("complete: have %d state %d, want 300 done", have, state)
	}

	if _, state := syncthingProgress(&Extract{Path: dir}, start); state != syncDone {
		t.Fatalf("unknown size: state %d, want done", state)
	}
}

func TestSyncthingProgressSingleFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "single.rar")
	writeSized(t, file, 50)

	if have, state := syncthingProgress(&Extract{Path: file, Size: 50}, time.Now()); state != syncDone || have != 50 {
		t.Fatalf("single file download: have %d state %d, want 50 done", have, state)
	}

	if have, state := syncthingProgress(&Extract{Path: file, Size: 60}, time.Now()); state != syncWaiting || have != 50 {
		t.Fatalf("short single file download: have %d state %d, want 50 waiting", have, state)
	}
}

func newSyncthingTestItem(t *testing.T, size uint64) (*Unpackerr, *Extract) {
	t.Helper()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "show.rar"), 100) // first volume here, later ones are not.

	unpack := New()
	unpack.StartDelay.Duration = 0
	unpack.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: 1,
		Suffix:   suffix,
		Logger:   unpack.Logger,
		FileMode: defaultFileMode,
		DirMode:  defaultDirMode,
	})
	t.Cleanup(unpack.Stop)

	item := &Extract{
		App:       starr.Sonarr,
		Path:      dir,
		Status:    WAITING,
		Updated:   time.Now().Add(-time.Hour),
		Syncthing: true,
		Size:      size,
		XProg:     &ExtractProgress{},
	}
	item.XProg.Extract = item
	unpack.Map["Show.Name"] = item

	return unpack, item
}

// A retry puts a failed item back to WAITING. It must pass the size check again,
// so an attempt that failed on a half-delivered set does not run again early.
func TestExtractCompletedDownloadWaitsForSyncthingSize(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 300)
	item.Retries = 1
	now := time.Now()

	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("short download: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

	if item.Status != QUEUED {
		t.Fatalf("stalled download: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadSyncthingSizeReached(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 100)
	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("complete download: status %s, want queued", item.Status)
	}
}

// After a restart, restored items have no Size until the Starr queue is polled.
// Extraction checks wait for that poll, and the poll fills in Size.
func TestRestoredSyncthingItemGetsSizeBeforeExtract(t *testing.T) {
	t.Parallel()

	const url = "http://sonarr:8989"

	unpack, item := newSyncthingTestItem(t, 0) // restored rows carry no size.
	item.URL = url
	unpack.Sonarr = instanceMap([]*SonarrConfig{{
		URL:       url,
		Protocols: defaultProtocol,
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title:    "Show.Name",
			Status:   "completed",
			Protocol: starr.Protocol("torrent"),
			Size:     300,
		}}},
	}})

	now := time.Now()

	unpack.extractCompletedDownloads(now) // not polled yet in this process.

	if item.Status != WAITING {
		t.Fatalf("before the first poll: status %s, want waiting", item.Status)
	}

	unpack.Sonarr["0"].polled = true
	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, now)

	if item.Size != 300 {
		t.Fatalf("Size after poll: %d, want 300", item.Size)
	}

	unpack.extractCompletedDownloads(now)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("after the poll: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}
