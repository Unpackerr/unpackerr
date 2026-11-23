package unpackerr

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/starr/sonarr"
	"golift.io/xtractr"
)

var errLateArchive = errors.New("late archive failed")

// newRescanItem is a Syncthing item the Starr app still lists, with a real
// extract queue. Nothing reads the callback channels in the background: a
// test hands each response to the handlers itself, with finishExtract.
func newRescanItem(t *testing.T, dir string) (*Unpackerr, *Extract) {
	t.Helper()

	const url = "http://sonarr:8989"

	unpack := New()
	unpack.StartDelay.Duration = 0
	unpack.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: 1,
		Suffix:   suffix,
		Logger:   unpack.Logger,
		FileMode: defaultFileMode,
		DirMode:  defaultDirMode,
	})

	t.Cleanup(func() { stopRescanQueue(unpack) })

	item := &Extract{
		App:       starr.Sonarr,
		URL:       url,
		Path:      dir,
		Status:    WAITING,
		Updated:   time.Now().Add(-time.Hour),
		Syncthing: true,
		XProg:     &ExtractProgress{},
	}
	item.XProg.Extract = item
	unpack.Map["Show.Name"] = item
	unpack.Sonarr = instanceMap([]*SonarrConfig{{
		URL:       url,
		Protocols: defaultProtocol,
		Queue: &sonarr.Queue{Records: []*sonarr.QueueRecord{{
			Title: "Show.Name", Status: "completed", Protocol: starr.Protocol("torrent"),
		}}},
	}})
	unpack.Sonarr["0"].polled = true

	return unpack, item
}

// stopRescanQueue stops the extract queue. A test that ends early leaves an
// extract running, and its worker waits on the unbuffered progress channel.
// Read both channels until the queue has stopped.
func stopRescanQueue(unpack *Unpackerr) {
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

	unpack.Stop()
	close(done)
}

// finishExtract hands the responses of the queued extract to the handlers,
// the way the main loop does, and returns the last one.
func finishExtract(t *testing.T, unpack *Unpackerr) *xtractr.Response {
	t.Helper()

	timeout := time.NewTimer(time.Minute)
	defer timeout.Stop()

	for {
		select {
		case resp := <-unpack.updates:
			unpack.handleXtractrCallback(resp)

			if resp.Done {
				return resp
			}
		case prog := <-unpack.progChan:
			unpack.handleProgress(prog)
		case <-timeout.C:
			t.Fatal("timed out waiting for the extract to finish")
		}
	}
}

// rescanPass runs one pass of the extract loop and returns how many archives
// it extracted. When the pass queues the item, that extract runs to the end.
func rescanPass(t *testing.T, unpack *Unpackerr, item *Extract, now time.Time) int {
	t.Helper()

	unpack.extractCompletedDownloads(now)

	if item.Status != QUEUED {
		return 0
	}

	return finishExtract(t, unpack).Archives.Count()
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()

	files := make(map[string][]byte, len(entries))
	for name, data := range entries {
		files[name] = []byte(data)
	}

	data, err := buildZip(files)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, defaultFileMode); err != nil {
		t.Fatal(err)
	}
}

func sortedFiles(dir string) []string {
	files := listFiles(dir)
	slices.Sort(files)

	return files
}

// A late archive that fails must not send the retry back over the archives
// already unpacked. The retry takes the late archive alone: nothing is
// refused, nothing becomes a remnant, and one failure costs one retry.
func TestRescanRetryKeepsFirstExtract(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	subs, movie := filepath.Join(dir, "subs.zip"), filepath.Join(dir, "movie.zip")
	writeZip(t, subs, map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	if item.Status != EXTRACTED || !slices.Equal(item.Unpacked, []string{subs}) || item.PreFiles != nil {
		t.Fatalf("first extract: status %s unpacked %v snapshot %v", item.Status, item.Unpacked, item.PreFiles)
	}

	writeSized(t, movie, 64) // the late archive arrives damaged.
	rescanPass(t, unpack, item, now)

	if item.Status != EXTRACTFAILED || !slices.Equal(item.Unpacked, []string{subs}) ||
		!slices.Equal(item.Resp.NewFiles, []string{filepath.Join(dir, "subs.srt")}) {
		t.Fatalf("failed late archive: status %s unpacked %v files %v", item.Status, item.Unpacked, item.Resp.NewFiles)
	}

	writeZip(t, movie, map[string]string{"movie.mkv": "movie"})
	unpack.checkExtractDone(now.Add(time.Hour))

	if count := rescanPass(t, unpack, item, now.Add(time.Hour)); count != 1 || item.Retries != 1 {
		t.Fatalf("retry: %d archives extracted after %d retries, want the late archive alone after 1", count, item.Retries)
	}

	want := []string{"movie.mkv", "movie.zip", "subs.srt", "subs.zip"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || !slices.Equal(got, want) {
		t.Fatalf("retry: status %s files %v, want %v", item.Status, got, want)
	}

	if !slices.Equal(item.Unpacked, []string{subs, movie}) || len(item.Resp.NewFiles) != 2 || item.Resp.Error != nil {
		t.Fatalf("retry record: unpacked %v files %v error %v", item.Unpacked, item.Resp.NewFiles, item.Resp.Error)
	}
}

// A late archive is a new extract with a retry budget of its own. The retries
// an earlier extract used do not count against it.
func TestLateArchiveGetsItsOwnRetries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	movie := filepath.Join(dir, "movie.zip")
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	item.Retries = unpack.maxRetries() // the first extract needed every retry.

	writeSized(t, movie, 64) // the late archive arrives damaged.
	rescanPass(t, unpack, item, now)
	unpack.checkExtractDone(now.Add(time.Hour))

	if item.Status != WAITING || item.Retries != 1 || item.NoRetry {
		t.Fatalf("failed late archive: status %s retries %d no retry %v, want a first retry",
			item.Status, item.Retries, item.NoRetry)
	}

	writeZip(t, movie, map[string]string{"movie.mkv": "movie"})

	if count := rescanPass(t, unpack, item, now.Add(time.Hour)); count != 1 || item.Status != EXTRACTED {
		t.Fatalf("retry: %d archives extracted, status %s, want the late archive extracted", count, item.Status)
	}
}

// An archive packed inside another one is left in the download folder by the
// extract that unpacked it. It is that extract's output: a later pass must
// not take it for a late arrival, or count it as delivered data.
func TestRescanSkipsEarlierOutput(t *testing.T) {
	t.Parallel()

	inner, err := buildZip(map[string][]byte{"nested.txt": []byte("inner")})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	outer := filepath.Join(dir, "outer.zip")
	writeZip(t, outer, map[string]string{"inner.zip": string(inner), "root.txt": "outer"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	want := []string{"inner.zip", "nested.txt", "outer.zip", "root.txt"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || !slices.Equal(got, want) {
		t.Fatalf("first extract: status %s files %v, want %v", item.Status, got, want)
	}

	if count := rescanPass(t, unpack, item, now); count != 0 || item.Status != EXTRACTED || item.Retries != 0 {
		t.Fatalf("second pass: %d archives extracted, status %s retries %d, want nothing new",
			count, item.Status, item.Retries)
	}

	if got := sortedFiles(dir); !slices.Equal(got, want) {
		t.Fatalf("second pass: files %v, want %v", got, want)
	}

	info, err := os.Stat(outer)
	if err != nil {
		t.Fatal(err)
	}

	files, scan := findDownloadArchives(item)
	if len(files) != 0 || scan.have != uint64(max(info.Size(), 0)) {
		t.Fatalf("search after the extract: archives %v, %d bytes counted, want the %d of outer.zip alone",
			files, scan.have, info.Size())
	}
}

// A file named like a later volume among the output is not a set that is
// still arriving. It must not hold a late archive back.
func TestRescanIgnoresVolumeNamesInOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "pack.zip"), map[string]string{"disc.r01": "volume", "readme.txt": "pack"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	writeZip(t, filepath.Join(dir, "extras.zip"), map[string]string{"extra.txt": "extra"})
	unpack.extractCompletedDownloads(now)

	if item.Status != QUEUED {
		t.Fatalf("late archive beside an extracted volume name: status %s note %q, want queued",
			item.Status, item.Note)
	}

	finishExtract(t, unpack)

	want := []string{"disc.r01", "extra.txt", "extras.zip", "pack.zip", "readme.txt"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || !slices.Equal(got, want) {
		t.Fatalf("late archive: status %s files %v, want %v", item.Status, got, want)
	}
}

// The search that decides whether to queue uses the extract's own list of
// archive types. A file the extract leaves alone must not queue an extract
// that then finds nothing to do.
func TestRescanIgnoresExcludedTypes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "subs"})
	writeSized(t, filepath.Join(dir, "album.cue"), 20)

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	if count := rescanPass(t, unpack, item, now); count != 0 || item.Status != EXTRACTED || item.Resp.Error != nil {
		t.Fatalf("cue sheet beside an extracted zip: %d archives extracted, status %s error %v",
			count, item.Status, item.Resp.Error)
	}
}

// Each new extract of an item takes its own snapshot. Files that arrived or
// were extracted since the last one are download content: a late archive
// that holds the same names leaves them alone.
func TestRescanSnapshotProtectsEarlierFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "first"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	// Syncthing delivers a loose file, then an archive that repeats both names.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("delivered"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	writeZip(t, filepath.Join(dir, "extras.zip"), map[string]string{
		"subs.srt": "second", "notes.txt": "archived", "extra.txt": "extra",
	})
	rescanPass(t, unpack, item, now)

	want := []string{"extra.txt", "extras.zip", "notes.txt", "subs.srt", "subs.zip"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || item.Retries != 0 || !slices.Equal(got, want) {
		t.Fatalf("late archive: status %s retries %d files %v, want %v", item.Status, item.Retries, got, want)
	}

	for name, want := range map[string]string{"subs.srt": "first", "notes.txt": "delivered"} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != want {
			t.Fatalf("%s holds %q (%v), want %q", name, got, err, want)
		}
	}
}

// A failed extract keeps its snapshot for the retry. What the failed attempt
// left behind is not in it, so the retry can still tell that apart.
func TestRescanRetryKeepsItsSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	writeSized(t, filepath.Join(dir, "movie.zip"), 64) // not a zip: this extract fails.
	rescanPass(t, unpack, item, now)

	if _, kept := item.PreFiles[filepath.Join(dir, "subs.srt")]; item.Status != EXTRACTFAILED || !kept {
		t.Fatalf("failed late archive: status %s, first extract's output in the snapshot: %v", item.Status, kept)
	}

	writeSized(t, filepath.Join(dir, "leftover.mkv"), 10)
	unpack.checkExtractDone(now.Add(time.Hour))
	unpack.extractCompletedDownloads(now.Add(time.Hour))

	if _, retaken := item.PreFiles[filepath.Join(dir, "leftover.mkv")]; item.Status != QUEUED || retaken {
		t.Fatalf("retry: status %s, snapshot taken again: %v", item.Status, retaken)
	}
}

// An item with earlier output is not queued without a snapshot: remnant
// handling could not tell that output from leftovers. A first extract has
// nothing to protect yet and queues as before.
func TestRescanWaitsForSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocked := filepath.Join(writeRemnant(t, dir, "plain.txt"), "Sub") // a folder under a file cannot be listed.
	files := xtractr.ArchiveList{blocked: {filepath.Join(blocked, "late.zip")}}

	unpack, item := newSyncthingItem(t, dir, 0)
	item.Status = EXTRACTED
	item.Unpacked = []string{filepath.Join(dir, "subs.zip")}
	unpack.queueDownloadExtract("Show.Name", item, files, time.Now())

	if item.Status != EXTRACTED || item.PreFiles != nil {
		t.Fatalf("no snapshot for a late archive: status %s snapshot %v, want it left extracted", item.Status, item.PreFiles)
	}

	item.Status, item.Unpacked = WAITING, nil
	unpack.queueDownloadExtract("Show.Name", item, files, time.Now())

	if item.Status != QUEUED {
		t.Fatalf("no snapshot for a first extract: status %s, want queued", item.Status)
	}
}

// The start of an ordinary retry replaces the failed attempt's response, so
// its notification carries no old error, time, or counts. The start of an
// extract for an item with earlier extracts keeps what those recorded.
func TestExtractStartResponse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zip := filepath.Join(dir, "subs.zip")
	failed := &xtractr.Response{
		Done: true, Error: errLateArchive, Size: 8,
		NewFiles: []string{filepath.Join(dir, "subs.srt")},
		Archives: xtractr.ArchiveList{dir: {zip}},
	}
	start := &xtractr.Response{
		X:        &xtractr.Xtract{Name: "Show.Name", Path: dir},
		Archives: xtractr.ArchiveList{dir: {filepath.Join(dir, "movie.zip")}},
		Started:  time.Now(),
	}

	unpack, item := newSyncthingItem(t, dir, 0)
	item.Status, item.Resp = QUEUED, failed

	unpack.handleXtractrCallback(start)

	if item.Status != EXTRACTING || item.Resp != start {
		t.Fatalf("retry start: status %s response %+v, want the start response", item.Status, item.Resp)
	}

	item.Status, item.Resp, item.Unpacked = QUEUED, failed, []string{zip}

	unpack.handleXtractrCallback(start)

	if item.Resp == start || item.Resp.Error != nil || !item.Resp.Started.Equal(start.Started) ||
		item.Resp.Size != 8 || len(item.Resp.NewFiles) != 1 || item.Resp.Archives.Count() != 2 {
		t.Fatalf("follow-up start: response %+v", item.Resp)
	}
}

// A finished extract leaves Retries at 0. Its late archives still get the
// volume check with every reported byte on disk, and the files it wrote are
// not counted toward that size.
func TestRescanChecksVolumesWithEveryByte(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zip, late := filepath.Join(dir, "subs.zip"), filepath.Join(dir, "late.7z.001")
	writeSized(t, zip, 10)
	writeSized(t, filepath.Join(dir, "subs.srt"), 50)
	write7zVolume(t, late, 200) // 32 bytes are here, the rest of the set is not.

	unpack, item := newSyncthingItem(t, dir, 42)
	item.Status = EXTRACTED
	item.Unpacked = []string{zip}
	item.Resp = &xtractr.Response{NewFiles: []string{filepath.Join(dir, "subs.srt")}}

	now := time.Now()
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != EXTRACTED || item.Note != noteWaitingSyncthing || item.SyncSeen != 42 {
		t.Fatalf("short late set: status %s note %q, %d bytes counted, want extracted, waiting, 42",
			item.Status, item.Note, item.SyncSeen)
	}

	write7zVolume(t, late, 32)
	unpack.extractCompletedDownload("Show.Name", now, item)

	if item.Status != QUEUED {
		t.Fatalf("whole late set: status %s note %q, want queued", item.Status, item.Note)
	}
}

// Archive identity is the exact path: Episode.zip is another archive than
// episode.zip on a filesystem that keeps both. Only the match from a later
// volume to its first volume ignores case, as the finder does.
func TestUnpackedSetArchiveIdentity(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("dl", "Show")
	lower, upper := filepath.Join(dir, "episode.zip"), filepath.Join(dir, "Episode.zip")
	set := newUnpackedSet(&Extract{
		Syncthing: true,
		Path:      dir,
		Unpacked:  []string{lower, filepath.Join(dir, "Movie.RAR")},
	})

	if set.accept(xtractr.ArchiveCandidate{Path: lower}) ||
		set.accept(xtractr.ArchiveCandidate{Path: filepath.Join(dir, "Sub", "..", "episode.zip")}) {
		t.Fatal("an archive that was already unpacked was accepted")
	}

	if !set.accept(xtractr.ArchiveCandidate{Path: upper}) {
		t.Fatal("Episode.zip was taken for episode.zip")
	}

	merged := mergeArchiveLists(xtractr.ArchiveList{dir: {lower}}, xtractr.ArchiveList{dir: {upper, lower}})
	if got := merged[dir]; !slices.Equal(got, []string{lower, upper}) {
		t.Fatalf("merged archives: %v", got)
	}

	tail, _ := continuationVolume(filepath.Join(dir, "movie.r01"))
	scan := &syncScan{tails: []volumeTail{tail}, unpacked: set}

	if got, head := missingVolumeHead(scan, nil); got != "" {
		t.Fatalf("Movie.RAR was unpacked, but %s still waits for %s", got, head)
	}
}

// The queue worker searches the download again when the extract starts. An
// archive that arrived after the wait checks ran is left for the next pass.
func TestSyncthingExtractTakesOnlyCheckedArchives(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "first.zip"), map[string]string{"first.txt": "first"})

	unpack, item := newRescanItem(t, dir)
	hold := make(chan struct{})
	release := sync.OnceFunc(func() { close(hold) })
	t.Cleanup(release)

	// Keep the only worker busy, so the item's extract starts after the second archive lands.
	_, err := unpack.Extract(&xtractr.Xtract{Path: t.TempDir(), CBFunction: func(*xtractr.Response) { <-hold }})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	unpack.extractCompletedDownloads(now)
	writeZip(t, filepath.Join(dir, "second.zip"), map[string]string{"second.txt": "second"})
	release()
	finishExtract(t, unpack)

	want := []string{"first.txt", "first.zip", "second.zip"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || !slices.Equal(got, want) {
		t.Fatalf("first extract: status %s files %v, want %v", item.Status, got, want)
	}

	rescanPass(t, unpack, item, now)

	want = []string{"first.txt", "first.zip", "second.txt", "second.zip"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || !slices.Equal(got, want) {
		t.Fatalf("next pass: status %s files %v, want %v", item.Status, got, want)
	}
}

// A response that ends in a remnant restart did not finish. Its archives
// are extracted again, so they are not recorded as unpacked.
func TestRemnantRestartRecordsNothingUnpacked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocker := writeRemnant(t, dir, "movie.mkv")
	subs, srt := filepath.Join(dir, "subs.zip"), filepath.Join(dir, "subs.srt")

	unpack, item := newSyncthingItem(t, dir, 0)
	item.Status = EXTRACTING
	item.PreFiles = map[string]os.FileInfo{}
	item.Unpacked = []string{subs}
	item.Resp = &xtractr.Response{NewFiles: []string{srt}, Archives: xtractr.ArchiveList{dir: {subs}}}

	unpack.handleXtractrCallback(&xtractr.Response{
		Done:       true,
		X:          &xtractr.Xtract{Name: "Show.Name", Path: dir},
		Archives:   xtractr.ArchiveList{dir: {filepath.Join(dir, "movie.zip")}},
		FinalDests: map[string]string{dir: dir},
		Refused:    []xtractr.RefusedFile{{Src: filepath.Join(dir+suffix, "movie.mkv"), Dest: blocker}},
		Started:    time.Now(),
	})

	if item.Status != WAITING || item.Retries != 1 || item.PreFiles == nil {
		t.Fatalf("remnant restart: status %s retries %d snapshot %v", item.Status, item.Retries, item.PreFiles)
	}

	if !slices.Equal(item.Unpacked, []string{subs}) || !slices.Equal(item.Resp.NewFiles, []string{srt}) {
		t.Fatalf("remnant restart record: unpacked %v files %v", item.Unpacked, item.Resp.NewFiles)
	}
}

// An item waiting to retry a late archive already has output on disk. When
// the Starr app drops it, that output is cleaned up the way an import is.
func TestWaitingItemWithUnpackedIsImported(t *testing.T) {
	t.Parallel()

	const url = "http://sonarr:8989"

	unpack := New()
	unpack.Sonarr = instanceMap([]*SonarrConfig{{URL: url, Protocols: defaultProtocol, Queue: &sonarr.Queue{}}})
	unpack.Sonarr["0"].polled = true
	unpack.Map["Late.Retry"] = &Extract{
		App: starr.Sonarr, URL: url, Status: WAITING, Syncthing: true, XProg: &ExtractProgress{},
		Unpacked: []string{filepath.Join("dl", "late", "subs.zip")},
		Resp:     &xtractr.Response{NewFiles: []string{filepath.Join("dl", "late", "subs.srt")}},
	}
	unpack.Map["Never.Extracted"] = &Extract{
		App: starr.Sonarr, URL: url, Status: WAITING, Syncthing: true, XProg: &ExtractProgress{},
	}

	unpack.checkQueueChanges(time.Now())

	if item := unpack.Map["Late.Retry"]; item == nil || item.Status != IMPORTED {
		t.Fatalf("waiting item with earlier output: %+v, want imported", item)
	}

	if item := unpack.Map["Never.Extracted"]; item != nil {
		t.Fatalf("waiting item with no output: %+v, want it removed", item)
	}
}

// A retry that finds nothing left to extract goes back to EXTRACTED: the
// earlier output is still there to import, and the old error is gone.
func TestRetryWithNothingNewReturnsToExtracted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zip := filepath.Join(dir, "subs.zip")
	writeSized(t, zip, 10)

	unpack, item := newSyncthingItem(t, dir, 0)
	item.Retries = 1
	item.Unpacked = []string{zip}
	item.Resp = &xtractr.Response{NewFiles: []string{filepath.Join(dir, "subs.srt")}, Error: errLateArchive}

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != EXTRACTED || item.Note != "" || item.Resp.Error != nil || len(item.Resp.NewFiles) != 1 {
		t.Fatalf("nothing new for the retry: status %s note %q response %+v", item.Status, item.Note, item.Resp)
	}
}

// Deleting the extracted files ends the record of what was unpacked: a
// download the Starr app lists again is extracted from the start. When
// nothing is deleted the record stays.
func TestDeleteForgetsUnpacked(t *testing.T) {
	t.Parallel()

	const url = "http://sonarr:8989"

	dir := t.TempDir()
	unpack := New()
	unpack.Sonarr = instanceMap([]*SonarrConfig{{URL: url, Protocols: defaultProtocol, Queue: &sonarr.Queue{}}})
	unpack.Sonarr["0"].polled = true

	for name, delay := range map[string]time.Duration{"Deleted": time.Minute, "Kept": -time.Minute} {
		unpack.Map[name] = &Extract{
			App: starr.Sonarr, URL: url, Path: dir, Status: IMPORTED, Syncthing: true,
			DeleteDelay: delay,
			Updated:     time.Now().Add(-time.Hour),
			Unpacked:    []string{filepath.Join(dir, name+".zip")},
			Resp:        &xtractr.Response{NewFiles: []string{filepath.Join(dir, name+".srt")}},
			XProg:       &ExtractProgress{},
		}
	}

	unpack.checkExtractDone(time.Now())

	if item := unpack.Map["Deleted"]; item.Status != DELETED || item.Unpacked != nil {
		t.Fatalf("extracted files deleted: status %s unpacked %v, want none", item.Status, item.Unpacked)
	}

	if item := unpack.Map["Kept"]; item.Status != DELETED || len(item.Unpacked) != 1 {
		t.Fatalf("extracted files kept: status %s unpacked %v, want the record kept", item.Status, item.Unpacked)
	}
}

// The record of unpacked archives survives a restart. A row written before
// the record existed lists the first extract's archives as its originals,
// and a failed row lists archives that were only found.
func TestHistoryKeepsUnpacked(t *testing.T) {
	t.Parallel()

	const url = "http://sonarr:8989"

	subs := filepath.Join("dl", "show", "subs.zip")
	old := []string{filepath.Join("dl", "old", "old.rar"), filepath.Join("dl", "old", "old.r00")}
	unpack := restoreTestUnpackerr(t)
	unpack.maybeRecordHistory("show", &Extract{
		App: starr.Sonarr, URL: url, Path: filepath.Join("dl", "show"), Status: EXTRACTED, Updated: time.Now(),
		Syncthing: true, Unpacked: []string{subs},
		Resp: &xtractr.Response{NewFiles: []string{filepath.Join("dl", "show", "subs.srt")}},
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "old", Kind: string(starr.Sonarr), URL: url, Path: filepath.Join("dl", "old"), Status: EXTRACTED,
		Updated: time.Now(), Syncthing: true, OrigFiles: old, PreFiles: old[:1],
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "failed", Kind: string(starr.Sonarr), URL: url, Path: filepath.Join("dl", "failed"),
		Status: EXTRACTFAILED, Updated: time.Now(), Syncthing: true, Error: "boom", OrigFiles: old, PreFiles: old[:1],
	})
	unpack.restoreQueueFromHistory()

	if item := unpack.Map["show"]; item == nil || !slices.Equal(item.Unpacked, []string{subs}) {
		t.Fatalf("restored row: %+v, want its unpacked archives", item)
	}

	if item := unpack.Map["old"]; item == nil || !slices.Equal(item.Unpacked, old) || item.PreFiles != nil {
		t.Fatalf("row from before the record: %+v, want its originals as unpacked and no snapshot", item)
	}

	if item := unpack.Map["failed"]; item == nil || len(item.Unpacked) != 0 || len(item.PreFiles) != 1 {
		t.Fatalf("failed row: %+v, want nothing unpacked and its snapshot kept", item)
	}
}

// Without a record of what was unpacked there is nothing to skip, so a
// finished item is left alone. A search would extract everything again.
func TestExtractedWithoutUnpackedIsNotRescanned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zip := filepath.Join(dir, "subs.zip")
	writeZip(t, zip, map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	item.Status = EXTRACTED
	item.Resp = &xtractr.Response{Archives: xtractr.ArchiveList{dir: {zip}}}

	if unpack.rescans("Show.Name", item) {
		t.Fatal("finished item with no record is listed for another search")
	}

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if got := sortedFiles(dir); item.Status != EXTRACTED || len(got) != 1 {
		t.Fatalf("finished item with no record: status %s files %v, want it left alone", item.Status, got)
	}
}

// The stale sweep leaves a finished Syncthing download alone while the Starr
// app lists it: a late archive can arrive days after the first extract.
func TestStaleSweepLeavesRescannedItem(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	later := now.Add(staleItemTimeout + time.Hour)
	unpack.checkExtractDone(later)
	unpack.checkQueueChanges(later)

	if item.Status != EXTRACTED {
		t.Fatalf("a day after the first extract: status %s, want extracted", item.Status)
	}

	writeZip(t, filepath.Join(dir, "movie.zip"), map[string]string{"movie.mkv": "movie"})
	rescanPass(t, unpack, item, later)

	want := []string{"movie.mkv", "movie.zip", "subs.srt", "subs.zip"}
	if got := sortedFiles(dir); item.Status != EXTRACTED || item.Retries != 0 || !slices.Equal(got, want) {
		t.Fatalf("late archive after a day: status %s retries %d files %v, want %v", item.Status, item.Retries, got, want)
	}
}

// A failed attempt lists files in its temp folder, which is deleted, and can
// repeat paths already on record. Only what reached the download folder is
// added, once. The size grows only when an extract finishes.
func TestKeepUnpackedMergesResponses(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("dl", "Show")
	subs, moved := filepath.Join(dir, "subs.srt"), filepath.Join(dir, "Sub", "moved.mkv")
	item := &Extract{
		Syncthing: true,
		Path:      dir,
		Unpacked:  []string{filepath.Join(dir, "subs.zip")},
		Resp: &xtractr.Response{
			NewFiles: []string{subs}, Size: 5,
			Archives: xtractr.ArchiveList{dir: {filepath.Join(dir, "subs.zip")}},
		},
	}
	next := &xtractr.Response{
		Done: true, Error: errLateArchive, Size: 99, Output: dir + suffix,
		NewFiles: []string{moved, filepath.Join(dir+suffix, "partial.mkv"), subs},
		Archives: xtractr.ArchiveList{dir: {filepath.Join(dir, "movie.zip")}},
	}

	failed := keepUnpacked(item, next, false)
	if !slices.Equal(failed.NewFiles, []string{subs, moved}) || failed.Size != 5 ||
		failed.Archives.Count() != 2 || failed.Error == nil {
		t.Fatalf("failed attempt: %+v", failed)
	}

	if done := keepUnpacked(item, next, true); len(done.NewFiles) != 3 || done.Size != 104 {
		t.Fatalf("finished extract: %+v", done)
	}

	item.Unpacked = nil
	if got := keepUnpacked(item, next, true); got != next {
		t.Fatalf("item with nothing unpacked: %+v, want the new response as it is", got)
	}
}

// The whole case: subs.zip is extracted, then the movie's RAR set arrives one
// volume at a time, a later volume first.
func TestRescanExtractsLateRARSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeZip(t, filepath.Join(dir, "subs.zip"), map[string]string{"subs.srt": "subs"})

	unpack, item := newRescanItem(t, dir)
	now := time.Now()
	rescanPass(t, unpack, item, now)

	copyXtractrFixture(t, dir, "multivol.part2.rar")
	rescanPass(t, unpack, item, now)

	if item.Status != EXTRACTED || item.Note != "" {
		t.Fatalf("a later volume alone: status %s note %q, want extracted with no note", item.Status, item.Note)
	}

	copyXtractrFixture(t, dir, "multivol.part1.rar")
	rescanPass(t, unpack, item, now)

	if item.Status != EXTRACTED || item.Note != noteWaitingSyncthing {
		t.Fatalf("first volume of a short set: status %s note %q, want extracted and waiting", item.Status, item.Note)
	}

	copyXtractrFixture(t, dir, "multivol.part3.rar")
	copyXtractrFixture(t, dir, "multivol.part4.rar")

	if count := rescanPass(t, unpack, item, now); count != 4 || len(item.Unpacked) != 5 {
		t.Fatalf("whole set: %d volumes extracted, unpacked %v, want the 4 volumes after subs.zip", count, item.Unpacked)
	}

	files := sortedFiles(dir)
	if item.Status != EXTRACTED || item.Retries != 0 || item.Resp.Error != nil || len(files) <= 6 {
		t.Fatalf("whole set: status %s retries %d error %v files %v", item.Status, item.Retries, item.Resp.Error, files)
	}

	if count := rescanPass(t, unpack, item, now); count != 0 || !slices.Equal(sortedFiles(dir), files) {
		t.Fatalf("pass after the set: %d archives extracted, files %v, want nothing new", count, sortedFiles(dir))
	}
}
