package unpackerr

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

func TestExtractCompletedDownloadNotesNoArchives(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unpack := New()
	unpack.StartDelay.Duration = 0
	item := &Extract{
		App:     starr.Sonarr,
		Path:    dir,
		Status:  WAITING,
		Updated: time.Now().Add(-time.Minute),
		XProg:   &ExtractProgress{},
	}
	item.XProg.Extract = item

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Note != noteNoExtractable {
		t.Fatalf("note %q", item.Note)
	}

	got := unpack.queueFromExtract("Show.Name", item)
	if got.Progress != noteNoExtractable {
		t.Fatalf("progress %q", got.Progress)
	}
}

func TestExtractCompletedDownloadKeepsStartDelayQuiet(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.StartDelay.Duration = time.Hour
	item := &Extract{
		App:     starr.Sonarr,
		Path:    t.TempDir(),
		Status:  WAITING,
		Updated: time.Now(),
	}

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Note != "" {
		t.Fatalf("note during start delay: %q", item.Note)
	}
}

func TestQueueFromExtractFolderNoteDoesNotOverrideLastWrite(t *testing.T) {
	t.Parallel()

	item := &Extract{
		App:     FolderString,
		Status:  WAITING,
		Updated: time.Now(),
		Note:    noteNoExtractable,
	}

	got := New().queueFromExtract("/watch/a", item)
	if got.Progress != "last write" {
		t.Fatalf("progress %q", got.Progress)
	}

	if got.Note != noteNoExtractable {
		t.Fatalf("note %q", got.Note)
	}
}

func TestExtractCompletedDownloadNotesWaitingSyncthing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "show.rar"), []byte("rar"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "syncthing.tmp"), []byte("tmp"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.StartDelay.Duration = 0
	item := &Extract{
		App:       starr.Sonarr,
		Path:      dir,
		Status:    WAITING,
		Updated:   time.Now().Add(-time.Minute),
		Syncthing: true,
		Note:      noteNoExtractable,
		XProg:     &ExtractProgress{},
	}
	item.XProg.Extract = item

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Note != noteWaitingSyncthing {
		t.Fatalf("note %q", item.Note)
	}

	if item.Status != WAITING {
		t.Fatalf("status %s", item.Status)
	}

	got := unpack.queueFromExtract("Show.Name", item)
	if got.Progress != noteWaitingSyncthing {
		t.Fatalf("progress %q", got.Progress)
	}
}

func TestStarrArchiveTypesAndAPEOpts(t *testing.T) {
	t.Parallel()

	plain := starrArchiveTypes(nil)
	if slices.Contains(plain, ".cue") {
		t.Fatalf("cue should stay off until a split flag is set: %v", plain)
	}

	withCue := &Extract{SplitFlac: true, APEFormat: "flac", APECompression: 3000}
	if !slices.Contains(starrArchiveTypes(withCue), ".cue") {
		t.Fatal("split_flac should include cue sheets")
	}

	opt := apeOpts(withCue)
	if opt.Output != xtractr.AudioFormatFLAC || opt.Compression != 3000 {
		t.Fatalf("opts %+v", opt)
	}

	if !lidarrImportsTracks(withCue) || lidarrImportsTracks(&Extract{}) {
		t.Fatal("import gate")
	}

	for _, name := range []string{"01 - Song.flac", "02 - Song.ape", "003 - Song.WAV"} {
		if !numberedTrackPattern.MatchString(name) {
			t.Fatalf("pattern missed %s", name)
		}
	}

	if numberedTrackPattern.MatchString("cover.jpg") {
		t.Fatal("pattern matched a non-track")
	}
}
