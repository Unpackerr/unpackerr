package unpackerr

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
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

	if state := syncthingProgress(item, scan, time.Now()); state != syncReached || scan.temp != "" || scan.have != 150 {
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
	if state := syncthingProgress(item, scan, later.Add(2*syncthingStall)); state != syncReached || scan.have != 300 {
		t.Fatalf("complete: have %d state %d, want 300 done", scan.have, state)
	}

	if state := syncthingProgress(&Extract{Path: dir}, scanFor(dir), start); state != syncUnknown {
		t.Fatalf("unknown size: state %d, want unknown", state)
	}
}

func TestSyncthingProgressSingleFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "single.rar")
	writeSized(t, file, 50)

	scan := scanFor(file)
	state := syncthingProgress(&Extract{Path: file, Size: 50}, scan, time.Now())

	if state != syncReached || scan.have != 50 {
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

	return newSyncthingItem(t, dir, size)
}

func newSyncthingItem(t *testing.T, dir string, size uint64) (*Unpackerr, *Extract) {
	t.Helper()

	unpack := New()
	unpack.StartDelay.Duration = 0
	// A finished archive reports progress on an unbuffered channel. Drain it so
	// the queue worker can finish and Stop can return.
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-unpack.progChan:
			case <-unpack.updates:
			case <-done:
				return
			}
		}
	}()

	t.Cleanup(func() { close(done) })

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

// A queue poll copies the instance stall window onto the matching item, and a
// later poll picks up a change. An item for another instance keeps its window.
func TestCheckStarrQueueRefreshesSyncthingStall(t *testing.T) {
	t.Parallel()

	const (
		tvURL  = "http://sonarr:8989"
		uhdURL = "http://sonarr-uhd:8989"
	)

	unpack := New()
	show := &Extract{App: starr.Sonarr, URL: tvURL, Status: WAITING}
	movie := &Extract{App: starr.Sonarr, URL: uhdURL, Status: WAITING, SyncStall: 15 * time.Minute}
	unpack.Map["Show.TV"] = show
	unpack.Map["Show.UHD"] = movie

	tvApp := &SonarrConfig{
		URL: tvURL, Protocols: defaultProtocol,
		SyncthingStall: cnfg.Duration{Duration: time.Minute},
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title: "Show.TV", Status: "completed", Protocol: starr.Protocol("torrent"),
		}}},
	}
	uhd := &SonarrConfig{
		URL: uhdURL, Protocols: defaultProtocol,
		SyncthingStall: cnfg.Duration{Duration: 15 * time.Minute},
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title: "Show.UHD", Status: "completed", Protocol: starr.Protocol("torrent"),
		}}},
	}
	unpack.Sonarr = instanceMap([]*SonarrConfig{tvApp, uhd})

	now := time.Now()
	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, now)

	if show.SyncStall != time.Minute || movie.SyncStall != 15*time.Minute {
		t.Fatalf("after the first poll: show %s movie %s, want 1m and 15m", show.SyncStall, movie.SyncStall)
	}

	tvApp.SyncthingStall.Duration = 2 * time.Hour

	checkStarrQueue(unpack, unpack.Sonarr, starr.Sonarr, now)

	if show.SyncStall != 2*time.Hour || movie.SyncStall != 15*time.Minute {
		t.Fatalf("after the stall change: show %s movie %s, want 2h and 15m", show.SyncStall, movie.SyncStall)
	}
}

// write7zVolume writes a 32-byte .7z.001 start header. recorded is the set
// length the header claims. CheckVolumes treats a claim past those 32 bytes
// as a missing volume, and a claim of 32 as a whole set.
func write7zVolume(t *testing.T, path string, recorded uint64) {
	t.Helper()

	const headerLen = 32

	if recorded < headerLen {
		t.Fatalf("7z volume shorter than a start header: recorded %d", recorded)
	}

	header := make([]byte, headerLen)
	copy(header, []byte("7z\xbc\xaf\x27\x1c"))
	binary.LittleEndian.PutUint64(header[20:28], recorded-headerLen)
	binary.LittleEndian.PutUint32(header[8:12], crc32.ChecksumIEEE(header[12:]))

	if err := os.WriteFile(path, header, defaultFileMode); err != nil {
		t.Fatal(err)
	}
}

func TestExtractCompletedDownloadStallsOnIncompleteSet(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 1000)
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 200)

	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item) // starts the stall clock.

	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("stalled incomplete set: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.Name", now.Add(2*syncthingStall), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("still stalled: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}

func TestExtractCompletedDownloadStallsOnWholeSet(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 1000)
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 32)

	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)
	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

	if item.Status != QUEUED {
		t.Fatalf("stalled whole set: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadUnknownSizeIncompleteSet(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 0)
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 200)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("unknown size, incomplete set: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}

func TestExtractCompletedDownloadFirstAttemptSkipsSetCheck(t *testing.T) {
	t.Parallel()

	// show.rar is 100 bytes and the volume header is 32, so the reported size is on disk.
	unpack, item := newSyncthingTestItem(t, 132)
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 200)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("first attempt with every byte: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadRetryWaitsOnLeftoverBytes(t *testing.T) {
	t.Parallel()

	// The leftover file is what makes the byte count reach the reported size.
	unpack, item := newSyncthingTestItem(t, 182)
	item.Retries = 1
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 200)
	writeSized(t, filepath.Join(item.Path, "Show.S01E01.mkv"), 50)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("retry with leftover bytes: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}

func TestExtractCompletedDownloadRetryQueuesWholeSet(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 132)
	item.Retries = 1
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 32)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("retry with a whole set: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadShortStallWindow(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 300)
	item.SyncStall = time.Second
	now := time.Now()

	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("inside a short stall window: status %s note %q, want waiting", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.Name", now.Add(time.Second), item)

	if item.Status != QUEUED {
		t.Fatalf("after the short stall window: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadSyncthingOffQueuesIncompleteSet(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 132)
	item.Syncthing = false
	write7zVolume(t, filepath.Join(item.Path, "show.7z.001"), 200)

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("syncthing off: status %s, want queued", item.Status)
	}
}

// xtractrDir is the module cache copy of golift.io/xtractr, which ships the
// multivol RAR fixtures CheckVolumes was written against.
func xtractrDir(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(context.Background(),
		"go", "list", "-m", "-f", "{{.Dir}}", "golift.io/xtractr").Output()
	if err != nil {
		t.Fatal(err)
	}

	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Fatal("golift.io/xtractr module dir is empty")
	}

	return dir
}

func copyXtractrFixture(t *testing.T, dir, name string) uint64 {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(xtractrDir(t), "test_data", name))
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(dir, name), data, defaultFileMode) //nolint:gosec // name is a fixture from this test.
	if err != nil {
		t.Fatal(err)
	}

	return uint64(len(data))
}

func TestExtractCompletedDownloadRetryWaitsOnShortRAR(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	size := copyXtractrFixture(t, dir, "multivol.part1.rar")
	unpack, item := newSyncthingItem(t, dir, size)
	item.Retries = 1

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("retry with a short RAR set: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}

func TestExtractCompletedDownloadRetryQueuesWholeRAR(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var size uint64

	for _, name := range []string{
		"multivol.part1.rar",
		"multivol.part2.rar",
		"multivol.part3.rar",
		"multivol.part4.rar",
	} {
		size += copyXtractrFixture(t, dir, name)
	}

	unpack, item := newSyncthingItem(t, dir, size)
	item.Retries = 1

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("retry with a whole RAR set: status %s, want queued", item.Status)
	}
}

func TestContinuationVolumeHeads(t *testing.T) {
	t.Parallel()

	tail, isCont := continuationVolume(filepath.Join("dl", "movie.r01"))
	if !isCont || tail.head != filepath.Join("dl", "movie.rar") || tail.alt != filepath.Join("dl", "movie.r00") {
		t.Fatalf("r01: continuation %v head %q alt %q", isCont, tail.head, tail.alt)
	}

	for _, name := range []string{"movie.r00", "movie.part01.rar", "movie.part1.rar", "movie.7z.001", "movie.7z.1000"} {
		if _, isCont := continuationVolume(filepath.Join("dl", name)); isCont {
			t.Errorf("%s is a head or not a volume, but was treated as a continuation", name)
		}
	}

	tail, isCont = continuationVolume(filepath.Join("dl", "movie.part02.rar"))
	if !isCont || tail.head != filepath.Join("dl", "movie.part01.rar") {
		t.Fatalf("part02: continuation %v head %q", isCont, tail.head)
	}

	tail, isCont = continuationVolume(filepath.Join("dl", "movie.part2.rar"))
	if !isCont || tail.head != filepath.Join("dl", "movie.part1.rar") {
		t.Fatalf("part2: continuation %v head %q", isCont, tail.head)
	}

	tail, isCont = continuationVolume(filepath.Join("dl", "movie.7z.002"))
	if !isCont || tail.head != filepath.Join("dl", "movie.7z.001") {
		t.Fatalf("7z.002: continuation %v head %q", isCont, tail.head)
	}
}

func TestMissingVolumeHeadSeesAnyPartOneWidth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tail, ok := continuationVolume(filepath.Join(dir, "movie.part02.rar"))

	if !ok {
		t.Fatal("part02 was not a continuation")
	}

	scan := &syncScan{tails: []volumeTail{tail}}
	files := xtractr.ArchiveList{dir: {filepath.Join(dir, "movie.part1.rar")}}

	if got, head := missingVolumeHead(scan, files); got != "" || head != "" {
		t.Fatalf("part1 present: missing %q head %q", got, head)
	}

	scan.unpacked = newUnpackedSet(&Extract{
		Syncthing: true,
		Path:      dir,
		Unpacked:  []string{filepath.Join(dir, "movie.part1.rar")},
	})
	files = nil

	if got, head := missingVolumeHead(scan, files); got != "" || head != "" {
		t.Fatalf("part1 already extracted: missing %q head %q", got, head)
	}
}

// A later volume whose first volume is not on disk holds the item while the
// download is still changing. After the stall window with no change the
// archives that are whole get extracted: a release that shipped without its
// first volume must not wait forever. A first volume that arrives after that
// is found by the rescan.
func TestExtractCompletedDownloadWaitsForLaterVolume(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"movie.r01", "movie.part02.rar", "movie.7z.002"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeSized(t, filepath.Join(dir, "subs.zip"), 10)
			writeSized(t, filepath.Join(dir, name), 10)

			// No reported size: only the missing first volume can hold this item.
			unpack, item := newSyncthingItem(t, dir, 0)
			now := time.Now()
			unpack.extractCompletedDownload("Show.Name", now, item)
			unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall-time.Second), item)

			if item.Status != WAITING || item.Note != noteWaitingSyncthing {
				t.Fatalf("later volume without its head: status %s note %q, want waiting", item.Status, item.Note)
			}

			unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

			if item.Status != QUEUED {
				t.Fatalf("no change for the stall window: status %s note %q, want queued", item.Status, item.Note)
			}
		})
	}
}

// A change on disk restarts the wait for a first volume, the same as it
// restarts the wait for the reported size.
func TestExtractCompletedDownloadLaterVolumeWaitRestartsOnChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "subs.zip"), 10)
	writeSized(t, filepath.Join(dir, "movie.r01"), 10)

	unpack, item := newSyncthingItem(t, dir, 0)
	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)

	writeSized(t, filepath.Join(dir, "movie.r02"), 10)
	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall-time.Second), item)
	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("another volume arrived: status %s note %q, want waiting", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.Name", now.Add(2*syncthingStall-time.Second), item)

	if item.Status != QUEUED {
		t.Fatalf("stall window after the last change: status %s note %q, want queued", item.Status, item.Note)
	}
}

// A retry with every reported byte on disk reads headers, so it also waits
// for a first volume, and for no longer than the stall window.
func TestExtractCompletedDownloadRetryWaitsForLaterVolume(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "subs.zip"), 10)
	writeSized(t, filepath.Join(dir, "movie.r01"), 10)

	unpack, item := newSyncthingItem(t, dir, 20)
	item.Retries = 1
	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("retry, later volume without its head: status %s note %q, want waiting", item.Status, item.Note)
	}

	unpack.extractCompletedDownload("Show.Name", now.Add(syncthingStall), item)

	if item.Status != QUEUED {
		t.Fatalf("retry, no change for the stall window: status %s note %q, want queued", item.Status, item.Note)
	}
}

// The movie set shows up one volume at a time after subs.zip. Once the first
// volume is present the volume check takes over. That wait has no cutoff, and
// the item queues when the set is whole.
func TestExtractCompletedDownloadQueuesWhenRARSetCompletes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSized(t, filepath.Join(dir, "subs.zip"), 10)
	copyXtractrFixture(t, dir, "multivol.part2.rar")

	unpack, item := newSyncthingItem(t, dir, 1<<30)
	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("part 2 only: status %s note %q, want waiting", item.Status, item.Note)
	}

	copyXtractrFixture(t, dir, "multivol.part1.rar")
	// Adding the head changes the byte count, so the stall clock starts over.
	unpack.extractCompletedDownload("Show.Name", now.Add(time.Minute), item)
	unpack.extractCompletedDownload("Show.Name", now.Add(time.Minute+2*syncthingStall), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("head present, set still short: status %s note %q, want waiting", item.Status, item.Note)
	}

	copyXtractrFixture(t, dir, "multivol.part3.rar")
	copyXtractrFixture(t, dir, "multivol.part4.rar")
	unpack.extractCompletedDownload("Show.Name", now.Add(time.Minute+2*syncthingStall), item)
	unpack.extractCompletedDownload("Show.Name", now.Add(time.Minute+3*syncthingStall), item)

	if item.Status != QUEUED {
		t.Fatalf("whole set: status %s, want queued", item.Status)
	}
}

func TestExtractCompletedDownloadRescansLateArchive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zip := filepath.Join(dir, "subs.zip")
	writeSized(t, zip, 20)

	unpack, item := newSyncthingItem(t, dir, 40)
	item.Status = EXTRACTED
	item.Unpacked = []string{zip}
	item.Resp = &xtractr.Response{
		NewFiles: []string{filepath.Join(dir, "subs.srt")},
		Archives: xtractr.ArchiveList{dir: {zip}},
		Size:     8,
	}

	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != EXTRACTED {
		t.Fatalf("nothing new yet: status %s, want extracted", item.Status)
	}

	movie := filepath.Join(dir, "movie.zip")
	writeSized(t, movie, 20)
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != QUEUED {
		t.Fatalf("late archive: status %s, want queued", item.Status)
	}

	assertLateExtractMerged(t, unpack, item, dir, movie, now)
}

func assertLateExtractMerged(t *testing.T, unpack *Unpackerr, item *Extract, dir, movie string, now time.Time) {
	t.Helper()

	unpack.handleXtractrCallback(&xtractr.Response{
		Done:     false,
		X:        &xtractr.Xtract{Name: "Show.Name", Path: dir},
		Archives: xtractr.ArchiveList{dir: {movie}},
		Started:  now,
	})

	if item.Status != EXTRACTING || len(item.Resp.NewFiles) != 1 {
		t.Fatalf("start of the second extract: status %s files %v", item.Status, item.Resp.NewFiles)
	}

	movieFile := filepath.Join(dir, "movie.mkv")
	unpack.handleXtractrCallback(&xtractr.Response{
		Done:     true,
		X:        &xtractr.Xtract{Name: "Show.Name", Path: dir},
		NewFiles: []string{movieFile},
		Archives: xtractr.ArchiveList{dir: {movie}},
		Size:     11,
		Started:  now,
	})

	subs := filepath.Join(dir, "subs.srt")
	if item.Status != EXTRACTED || item.Resp == nil || item.Resp.Size != 19 || len(item.Resp.NewFiles) != 2 ||
		item.Resp.NewFiles[0] != subs || item.Resp.NewFiles[1] != movieFile || item.Resp.Archives.Count() != 2 {
		t.Fatalf("merged response: status %s resp %+v", item.Status, item.Resp)
	}

	if want := []string{filepath.Join(dir, "subs.zip"), movie}; !slices.Equal(item.Unpacked, want) {
		t.Fatalf("unpacked archives: %v, want %v", item.Unpacked, want)
	}
}

func TestRescanDoesNotWaitForExtractedVolumeHead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	head := filepath.Join(dir, "movie.part01.rar")
	writeSized(t, head, 10)
	writeSized(t, filepath.Join(dir, "movie.part02.rar"), 10)
	writeSized(t, filepath.Join(dir, "extras.zip"), 10)

	unpack, item := newSyncthingItem(t, dir, 0)
	item.Status = EXTRACTED
	item.Unpacked = []string{head}
	item.Resp = &xtractr.Response{
		Archives: xtractr.ArchiveList{dir: {head}},
		NewFiles: []string{filepath.Join(dir, "movie.mkv")},
	}

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != QUEUED {
		t.Fatalf("new zip beside an already extracted set: status %s note %q, want queued", item.Status, item.Note)
	}
}

func TestExtractCompletedDownloadsRescansWhileStillQueued(t *testing.T) {
	t.Parallel()

	const url = "http://sonarr:8989"

	dir := t.TempDir()
	zip := filepath.Join(dir, "subs.zip")
	writeSized(t, zip, 20)

	unpack, item := newSyncthingItem(t, dir, 40)
	item.URL = url
	item.Status = EXTRACTED
	item.Unpacked = []string{zip}
	item.Resp = &xtractr.Response{
		NewFiles: []string{filepath.Join(dir, "subs.srt")},
		Archives: xtractr.ArchiveList{dir: {zip}},
	}

	server := &SonarrConfig{
		URL:       url,
		Protocols: defaultProtocol,
		Queue:     &sonarr.Queue{},
	}
	unpack.Sonarr = instanceMap([]*SonarrConfig{server})
	unpack.Sonarr["0"].polled = true

	now := time.Now()
	unpack.extractCompletedDownloads(now)

	if item.Status != EXTRACTED {
		t.Fatalf("imported item: status %s, want extracted", item.Status)
	}

	item.Syncthing = false
	server.Queue.Records = []*sonarr.QueueRecord{{
		Title: "Show.Name", Status: "completed", Protocol: starr.Protocol("torrent"),
	}}

	writeSized(t, filepath.Join(dir, "movie.zip"), 20)
	unpack.extractCompletedDownloads(now)

	if item.Status != EXTRACTED {
		t.Fatalf("syncthing off: status %s, want extracted", item.Status)
	}

	item.Syncthing = true

	unpack.extractCompletedDownloads(now)

	if item.Status != QUEUED {
		t.Fatalf("still in the Starr queue: status %s, want queued", item.Status)
	}
}
