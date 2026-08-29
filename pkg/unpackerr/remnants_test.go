package unpackerr

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func mustSnapshot(t *testing.T, paths ...string) map[string]os.FileInfo {
	t.Helper()

	snap, err := keepDirSnapshot(nil, paths...)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	return snap
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

	got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0)
	if !ok || got != WAITING {
		t.Fatalf("expected restart, got %v ok=%v", got, ok)
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
	unpack.RemnantAction = "delete"
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); !ok || got != WAITING {
		t.Fatalf("expected restart, got %v ok=%v", got, ok)
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

	snapshot := mustSnapshot(t, dest) // blocker present before extraction

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, snapshot, 0); ok {
		t.Fatalf("download content should not be touched, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("download file must remain, err=%v", err)
	}
}

func TestHandleRemnantsKeepsNestedDownloadContent(t *testing.T) {
	t.Parallel()

	unpack := New()
	root := t.TempDir()
	sub := filepath.Join(root, "CD1")

	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	blocker := writeRemnant(t, sub, "file_id.diz")
	archives := xtractr.ArchiveList{sub: {filepath.Join(sub, "movie.rar")}}
	snapshot := mustSnapshot(t, archiveSnapshotPaths(root, archives)...)

	resp := &xtractr.Response{
		FinalDests: map[string]string{sub: sub},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, snapshot, 0); ok {
		t.Fatalf("nested download content should not be touched, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("nested download file must remain, err=%v", err)
	}
}

func TestHandleRemnantsNestedLeftoverNotProtectedByRootBasename(t *testing.T) {
	t.Parallel()

	unpack := New()
	root := t.TempDir()
	sub := filepath.Join(root, "CD1")

	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeRemnant(t, root, "readme.txt") // download content at the search root
	leftover := writeRemnant(t, sub, "readme.txt")
	// Snapshot only the search root — the leftover lives in the archive dest.
	snapshot := mustSnapshot(t, root)

	resp := &xtractr.Response{
		FinalDests: map[string]string{sub: sub},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: leftover}},
	}

	if got, ok := unpack.handleRemnants(resp, snapshot, 0); !ok || got != WAITING {
		t.Fatalf("nested leftover must not match a root basename, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(leftover); !os.IsNotExist(err) {
		t.Fatalf("nested leftover should be renamed away, lstat err=%v", err)
	}
}

func TestHandleRemnantsCaseInsensitiveSnapshot(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	// Snapshot has "Movie.mkv"; refusal reports "movie.mkv" (same file on a
	// case-insensitive filesystem). isDownloadContent falls back to os.SameFile.
	writeRemnant(t, dest, "Movie.mkv")

	snapshot := mustSnapshot(t, dest)

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: filepath.Join(dest, "movie.mkv")}},
	}

	got, ok := unpack.handleRemnants(resp, snapshot, 0)
	// On a case-insensitive FS (macOS default) SameFile matches → no override.
	// On a case-sensitive FS "movie.mkv" does not exist → not download content,
	// and the file is renamed (WAITING). Either is correct; assert it is not Failed.
	if ok && got == EXTRACTFAILED {
		t.Fatalf("case-variant download file should not fail, got %v", got)
	}
}

func TestHandleRemnantsOffLeavesInPlaceAndFails(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.RemnantAction = "off"
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); !ok || got != EXTRACTFAILED {
		t.Fatalf("off should leave the blocker and fail, got %v ok=%v", got, ok)
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

	got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, unpack.MaxRetries)
	if !ok || got != EXTRACTFAILED {
		t.Fatalf("exhausted retries should fail, got %v ok=%v", got, ok)
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

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); !ok || got != WAITING {
		t.Fatalf("expected restart, got %v ok=%v", got, ok)
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

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); ok {
		t.Fatalf("empty dests should be a no-op, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("no-dests refusal must not touch the file, err=%v", err)
	}
}

func TestHandleRemnantsIgnoresOutputWithoutFinalDests(t *testing.T) {
	t.Parallel()

	unpack := New()
	output := t.TempDir()
	blocker := writeRemnant(t, output, "movie.mkv")

	resp := &xtractr.Response{
		Output:  output,
		Refused: []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); ok {
		t.Fatalf("Output is not a dest unless FinalDests says so, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("squash/temp Output refusal must not be cleared, err=%v", err)
	}
}

func TestHandleRemnantsNilSnapshotIsNoop(t *testing.T) {
	t.Parallel()

	unpack := New()
	dest := t.TempDir()
	blocker := writeRemnant(t, dest, "movie.mkv")

	resp := &xtractr.Response{
		FinalDests: map[string]string{dest: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: blocker}},
	}

	if got, ok := unpack.handleRemnants(resp, nil, 0); ok {
		t.Fatalf("nil snapshot cannot classify, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("nil snapshot must not touch the file, err=%v", err)
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

	if got, ok := unpack.handleRemnants(resp, map[string]os.FileInfo{}, 0); ok {
		t.Fatalf("refusal outside FinalDests should be ignored, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(blocker); err != nil {
		t.Fatalf("outside-dest refusal must not touch the file, err=%v", err)
	}
}

func TestKeepDirSnapshotReadDirFailure(t *testing.T) {
	t.Parallel()

	file := writeRemnant(t, t.TempDir(), "not-a-dir")
	out := map[string]os.FileInfo{}

	if err := keepDirChildren(out, file); err == nil {
		t.Fatal("a file dest must fail the snapshot, not look empty")
	}
}

func TestKeepDirSnapshotMissingDestIsEmpty(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "nope", "also-nope")

	snap, err := keepDirSnapshot(nil, missing)
	if err != nil || snap == nil {
		t.Fatalf("missing dest is empty, not a failed snapshot, snap=%v err=%v", snap, err)
	}

	if len(snap) != 0 {
		t.Fatalf("expected no children, got %v", snap)
	}
}

func TestArchiveSnapshotPathsIncludesArchiveDest(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "movie.rar")
	dest := strings.TrimSuffix(archive, filepath.Ext(archive))

	got := archiveSnapshotPaths(archive, xtractr.ArchiveList{archive: {archive}})
	if !slices.Contains(got, dest) {
		t.Fatalf("archive-file dest %s missing from %v", dest, got)
	}
}

func TestHandleRemnantsKeepsArchiveFileDestChildren(t *testing.T) {
	t.Parallel()

	unpack := New()
	parent := t.TempDir()
	archive := filepath.Join(parent, "movie.rar")

	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	dest := strings.TrimSuffix(archive, filepath.Ext(archive))
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	nfo := writeRemnant(t, dest, "movie.nfo")
	snapshot := mustSnapshot(t, archiveSnapshotPaths(archive, xtractr.ArchiveList{archive: {archive}})...)

	resp := &xtractr.Response{
		FinalDests: map[string]string{archive: dest},
		Refused:    []xtractr.RefusedFile{{Src: "x", Dest: nfo}},
	}

	if got, ok := unpack.handleRemnants(resp, snapshot, 0); ok {
		t.Fatalf("pre-existing dest child must be kept, got %v ok=%v", got, ok)
	}

	if _, err := os.Lstat(nfo); err != nil {
		t.Fatalf("download file in archive dest must remain, err=%v", err)
	}
}

func TestKeepDirSnapshotKeepsExisting(t *testing.T) {
	t.Parallel()

	existing := map[string]os.FileInfo{"sentinel": nil}

	snap, err := keepDirSnapshot(existing, t.TempDir())
	if err != nil {
		t.Fatalf("existing snapshot: %v", err)
	}

	if _, ok := snap["sentinel"]; !ok || len(snap) != 1 {
		t.Fatalf("retries must not recapture leftovers, got %v", snap)
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
