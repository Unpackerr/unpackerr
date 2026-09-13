package unpackerr

import (
	"testing"
	"time"

	"golift.io/starr"
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

	got := queueFromExtract("Show.Name", item)
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

	got := queueFromExtract("/watch/a", item)
	if got.Progress != "last write" {
		t.Fatalf("progress %q", got.Progress)
	}
}
