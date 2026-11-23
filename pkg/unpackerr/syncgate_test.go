package unpackerr

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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

// scanFor fills a scan the way extractCompletedDownload does: from the archive search.
func scanFor(path string) *syncScan {
	scan := &syncScan{}
	xtractr.FindCompressedFiles(xtractr.Filter{Path: path, Observe: scan.observe})
	scan.singleFile(path)

	return scan
}

func TestSyncthingTempNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	got := syncthingTempNames(filepath.Join(dir, "show.r01"))
	want := []string{filepath.Join(dir, ".syncthing.show.r01.tmp"), filepath.Join(dir, "~syncthing~show.r01.tmp")}

	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("short name: got %q, want %q", got, want)
	}

	// 145 bytes still fits: the temp name is 160 bytes.
	fits := strings.Repeat("a", 145)
	if got := syncthingTempNames(filepath.Join(dir, fits)); got[0] != filepath.Join(dir, ".syncthing."+fits+".tmp") {
		t.Fatalf("145 byte name must not be hashed: %q", got[0])
	}

	long := strings.Repeat("b", 146)
	sum := sha256.Sum256([]byte(long))
	hashed := hex.EncodeToString(sum[:])

	got = syncthingTempNames(filepath.Join(dir, long))
	want = []string{filepath.Join(dir, ".syncthing."+hashed+".tmp"), filepath.Join(dir, "~syncthing~"+hashed+".tmp")}

	if got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("146 byte name must be replaced by its SHA-256: %q", got)
	}
}

func TestIsSyncthingTemp(t *testing.T) {
	t.Parallel()

	// Every name syncthingTempNames builds must be recognized, hashed or not.
	for _, base := range []string{"show.r01", strings.Repeat("a", 145), strings.Repeat("a", 146)} {
		for _, temp := range syncthingTempNames(filepath.Join("dir", base)) {
			if !isSyncthingTemp(filepath.Base(temp)) {
				t.Errorf("Syncthing temp name not recognized: %q", filepath.Base(temp))
			}
		}
	}

	for _, name := range []string{
		"sample.tmp",             // plain .tmp content shipped with a download.
		"show.syncthing.r01.tmp", // prefix not at the start.
		".syncthing..tmp",        // empty name.
		"~syncthing~.tmp",        // empty name.
		".syncthing.show.r01",    // no suffix.
		".syncthing." + strings.Repeat("a", 146) + ".tmp", // too long: Syncthing would have hashed it.
	} {
		if isSyncthingTemp(name) {
			t.Errorf("not a Syncthing temp name, but recognized: %q", name)
		}
	}
}

func TestSyncthingPlainTmpIsContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "a.rar"), 100)
	writeSized(t, filepath.Join(dir, "Sub", "notes.tmp"), 50) // part of the release, not a sync in progress.

	scan := scanFor(dir)
	item := &Extract{Path: dir, Size: 150}

	if state := syncthingProgress(item, scan, time.Now()); state != syncDone || scan.temp != "" || scan.have != 150 {
		t.Fatalf("plain .tmp file: have %d temp %q state %d, want 150, none, done", scan.have, scan.temp, state)
	}
}

func TestSyncthingProgress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "a.rar"), 100)
	writeSized(t, filepath.Join(dir, "Sub", "a.r00"), 100) // subfolders count too.

	temp := filepath.Join(dir, "Sub", ".syncthing.a.r01.tmp")
	writeSized(t, temp, 100) // a temp file may already have its final size; it must not count.

	start := time.Now()
	item := &Extract{Path: dir, Size: 300}

	scan := scanFor(dir)
	if state := syncthingProgress(item, scan, start); state != syncTemp || scan.have != 200 || scan.temp != temp {
		t.Fatalf("temp file in a subfolder: have %d temp %q state %d, want 200, %q, temp", scan.have, scan.temp, state, temp)
	}

	// A temp file is progress, however long it stays: no stall while one exists.
	if state := syncthingProgress(item, scanFor(dir), start.Add(2*syncthingStall)); state != syncTemp {
		t.Fatalf("temp file present after the stall window: state %d, want temp", state)
	}

	// The temp file goes away without its data arriving, e.g. Syncthing gave up on it.
	if err := os.Remove(temp); err != nil {
		t.Fatal(err)
	}

	later := start.Add(2 * syncthingStall)

	if state := syncthingProgress(item, scanFor(dir), later.Add(syncthingStall-time.Second)); state != syncWaiting {
		t.Fatalf("short and inside the stall window: state %d, want waiting", state)
	}

	if state := syncthingProgress(item, scanFor(dir), later.Add(syncthingStall)); state != syncStalled {
		t.Fatalf("short with no change for the stall window: state %d, want stalled", state)
	}

	writeSized(t, filepath.Join(dir, "Sub", "a.r01"), 100)

	scan = scanFor(dir)
	if state := syncthingProgress(item, scan, later.Add(2*syncthingStall)); state != syncDone || scan.have != 300 {
		t.Fatalf("complete: have %d state %d, want 300 done", scan.have, state)
	}

	if state := syncthingProgress(&Extract{Path: dir}, scanFor(dir), start); state != syncDone {
		t.Fatalf("unknown size: state %d, want done", state)
	}
}

func TestSyncthingProgressSingleFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "single.rar")
	writeSized(t, file, 50)

	scan := scanFor(file)
	if state := syncthingProgress(&Extract{Path: file, Size: 50}, scan, time.Now()); state != syncDone || scan.have != 50 {
		t.Fatalf("single file download: have %d state %d, want 50 done", scan.have, state)
	}

	if state := syncthingProgress(&Extract{Path: file, Size: 60}, scanFor(file), time.Now()); state != syncWaiting {
		t.Fatalf("short single file download: state %d, want waiting", state)
	}

	// The archive search lists nothing for a file, so the temp names beside it are checked.
	for _, temp := range syncthingTempNames(file) {
		writeSized(t, temp, 10)

		scan := scanFor(file)

		state := syncthingProgress(&Extract{Path: file, Size: 50}, scan, time.Now())
		if state != syncTemp || scan.temp != temp {
			t.Fatalf("temp %s beside a single file: temp %q state %d, want temp", temp, scan.temp, state)
		}

		if err := os.Remove(temp); err != nil {
			t.Fatal(err)
		}
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

// Season packs often keep each episode in its own folder. A temp file there
// must hold the whole item, even when the reported size is already on disk.
func TestExtractCompletedDownloadWaitsForSyncthingTempInSubfolder(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 100)
	writeSized(t, filepath.Join(item.Path, "Show.S01E02", ".syncthing.show.s01e02.r00.tmp"), 0)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("temp file in a subfolder: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
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
