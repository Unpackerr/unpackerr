package unpackerr

import (
	"os"
	"path/filepath"
	"testing"

	"golift.io/xtractr"
)

// writeRemnant writes a file into dir and returns its path.
func writeRemnant(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("leftover"), 0o600); err != nil {
		t.Fatalf("write remnant: %v", err)
	}

	return path
}

func TestHandleRemnantsRenamesAndRestarts(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: filepath.Join(dest+"_x", "movie.mkv"), Dest: blocker}},
	}

	got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0)
	if got != remnantRestart {
		t.Fatalf("expected restart, got %v", got)
	}

	if _, err := os.Lstat(blocker); !os.IsNotExist(err) {
		t.Fatalf("blocker should be renamed away, lstat err=%v", err)
	}

	if _, err := os.Lstat(blocker + remnantSuffix); err != nil {
		t.Fatalf("expected renamed %s, err=%v", blocker+remnantSuffix, err)
	}
}

func TestHandleRemnantsDeletes(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.RemnantAction = remnantActionDelete
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); got != remnantRestart {
		t.Fatalf("expected restart, got %v", got)
	}

	if _, err := os.Lstat(blocker); !os.IsNotExist(err) {
		t.Fatalf("blocker should be deleted, lstat err=%v", err)
	}
}

func TestHandleRemnantsKeepsDownloadContent(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "file_id.diz")

	snapshot := keepDirSnapshot(nil, dest) // blocker present before extraction

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got := unpack.handleRemnants(resp, snapshot, 0); got != remnantNone {
		t.Fatalf("download content should not be touched, got %v", got)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("download file must remain, err=%v", err)
	}
}

func TestHandleRemnantsCaseInsensitiveSnapshot(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	// Snapshot has "Movie.mkv"; refusal reports "movie.mkv" (same file on a
	// case-insensitive filesystem). isDownloadContent falls back to os.SameFile.
	writeRemnant(t, dest, "Movie.mkv")

	snapshot := keepDirSnapshot(nil, dest)

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: filepath.Join(dest, "movie.mkv")}},
	}

	got := unpack.handleRemnants(resp, snapshot, 0)
	// On a case-insensitive FS (macOS default) SameFile matches → remnantNone.
	// On a case-sensitive FS "movie.mkv" does not exist → not download content,
	// and the file is renamed (restart). Either is correct; assert it is not Failed.
	if got == remnantFailed {
		t.Fatalf("case-variant download file should not fail, got %v", got)
	}
}

func TestHandleRemnantsOffLeavesInPlaceAndFails(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.RemnantAction = remnantActionOff
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); got != remnantFailed {
		t.Fatalf("off should leave the blocker and fail, got %v", got)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("remnant_action=off must leave the file, err=%v", err)
	}
}

func TestHandleRemnantsRetriesExhausted(t *testing.T) {
	t.Parallel()

	unpack := New() // default MaxRetries = defaultMaxRetries
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, unpack.MaxRetries)
	if got != remnantFailed {
		t.Fatalf("exhausted retries should fail, got %v", got)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("exhausted retries should not touch the file, err=%v", err)
	}
}

func TestHandleRemnantsRollbackPartialMove(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")
	moved := writeRemnant(t, dest, "movie.nfo") // a sibling xtractr already moved

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
		NewFiles:   []string{moved},
	}

	if got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); got != remnantRestart {
		t.Fatalf("expected restart, got %v", got)
	}

	if _, err := os.Lstat(moved); !os.IsNotExist(err) {
		t.Fatalf("partial-move sibling should be rolled back before restart, err=%v", err)
	}
}

func TestHandleRemnantsNoFinalDests(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{Refused: []xtractr.RefusedFile{{Src: "x", Dest: blocker}}}

	if got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); got != remnantNone {
		t.Fatalf("empty FinalDests should be a no-op, got %v", got)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("no-FinalDests refusal must not touch the file, err=%v", err)
	}
}

func TestHandleRemnantsIgnoresOutsideDest(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	other := t.TempDir()
	blocker := writeRemnant(t, other, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); got != remnantNone {
		t.Fatalf("refusal outside FinalDests should be ignored, got %v", got)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("outside-dest refusal must not touch the file, err=%v", err)
	}
}

func TestValidateRemnantAction(t *testing.T) {
	t.Parallel()

	for action, wantErr := range map[string]bool{
		"": false, "rename": false, "delete": false, "off": false,
		"RENAME": false, "Delete": false, "bogus": true,
	} {
		unpack := &Unpackerr{Config: &Config{RemnantAction: action}}

		err := unpack.validateRemnantAction()
		if (err != nil) != wantErr {
			t.Errorf("remnant_action=%q: err=%v, wantErr=%v", action, err, wantErr)
		}
	}
}
