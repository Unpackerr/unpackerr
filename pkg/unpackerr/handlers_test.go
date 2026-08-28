package unpackerr

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

func TestSnapshotDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("y"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := snapshotDir(dir)
	if len(got) != 2 {
		t.Fatalf("snapshotDir: expected 2 keys, got %d", len(got))
	}

	if got["a"] == nil || got["b"] == nil {
		t.Fatal("snapshotDir: missing file info")
	}
}

func testPreFiles(t *testing.T, path string) map[string]os.FileInfo {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}

	return map[string]os.FileInfo{filepath.Base(path): info}
}

func TestValidateRemnantAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr error
	}{
		{in: "", want: remnantActionRename},
		{in: "RENAME", want: remnantActionRename},
		{in: " delete ", want: remnantActionDelete},
		{in: "off", want: remnantActionOff},
		{in: "overwrite", wantErr: ErrInvalidRemnantAction},
	}

	for _, test := range tests {
		unpack := New()
		unpack.RemnantAction = test.in

		err := unpack.validateRemnantAction()
		if test.wantErr != nil {
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validateRemnantAction(%q): error = %v, want %v", test.in, err, test.wantErr)
			}

			continue
		}

		if err != nil {
			t.Fatalf("validateRemnantAction(%q): unexpected error: %v", test.in, err)
		}

		if unpack.RemnantAction != test.want {
			t.Fatalf("validateRemnantAction(%q): got %q, want %q", test.in, unpack.RemnantAction, test.want)
		}
	}
}

func TestRepairRemnants(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	remnant := filepath.Join(dir, "movie.mkv")
	keep := filepath.Join(dir, "file_id.diz")
	outside := filepath.Join(t.TempDir(), "other.mkv")

	for _, path := range []string{remnant, keep, outside} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	unpack := New()
	unpack.RemnantAction = remnantActionRename

	resp := &xtractr.Response{
		Refused: []xtractr.RefusedFile{
			{Src: "tmp/movie.mkv", Dest: remnant},
			{Src: "tmp2/movie.mkv", Dest: remnant}, // duplicate dest
			{Src: "tmp/file_id.diz", Dest: keep},
			{Src: "tmp/other.mkv", Dest: outside},
		},
	}

	got := unpack.repairRemnants(testPreFiles(t, keep), []string{dir}, resp)

	if got != 1 {
		t.Fatalf("repairRemnants: cleared %d, want 1", got)
	}

	if _, err := os.Stat(remnant); err == nil {
		t.Fatal("remnant dest should have been renamed")
	}

	if _, err := os.Stat(remnant + remnantSuffix); err != nil {
		t.Fatalf("expected renamed remnant: %v", err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("torrent-listed file should remain: %v", err)
	}

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("non-child dest should remain: %v", err)
	}
}

func TestRepairRemnantsSameFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keep := filepath.Join(dir, "Movie.mkv")
	alias := filepath.Join(dir, "alias.mkv")

	if err := os.WriteFile(keep, []byte("from torrent"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.Link(keep, alias); err != nil {
		t.Skipf("hardlink not supported: %v", err)
	}

	unpack := New()
	unpack.RemnantAction = remnantActionDelete
	got := unpack.repairRemnants(
		testPreFiles(t, keep),
		[]string{dir},
		&xtractr.Response{Refused: []xtractr.RefusedFile{{Dest: alias}}},
	)

	if got != 0 {
		t.Fatalf("SameFile dest should be kept, cleared %d", got)
	}

	if _, err := os.Stat(alias); err != nil {
		t.Fatalf("hard-link dest should remain: %v", err)
	}
}

func TestRepairRemnantsDelete(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")
	subdir := filepath.Join(dir, "extras")

	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.Mkdir(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	unpack := New()
	unpack.RemnantAction = remnantActionDelete
	resp := &xtractr.Response{
		Refused: []xtractr.RefusedFile{
			{Dest: file},
			{Dest: subdir},
		},
	}

	got := unpack.repairRemnants(nil, []string{dir}, resp)
	if got != 2 {
		t.Fatalf("repairRemnants delete: cleared %d, want 2", got)
	}

	if _, err := os.Stat(file); err == nil {
		t.Fatal("file remnant should have been deleted")
	}

	if _, err := os.Stat(subdir); err == nil {
		t.Fatal("dir remnant should have been deleted")
	}
}

func TestRepairRemnantsOff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")

	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	unpack.RemnantAction = remnantActionOff
	resp := &xtractr.Response{Refused: []xtractr.RefusedFile{{Dest: file}}}

	got := unpack.repairRemnants(nil, []string{dir}, resp)
	if got != 0 {
		t.Fatalf("repairRemnants off: cleared %d, want 0", got)
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("off must not touch the dest: %v", err)
	}
}

func TestRepairRemnantsNumericSuffix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")
	taken := file + remnantSuffix

	if err := os.WriteFile(file, []byte("new"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := os.WriteFile(taken, []byte("old"), 0o600); err != nil {
		t.Fatalf("write taken: %v", err)
	}

	unpack := New()
	unpack.RemnantAction = remnantActionRename
	got := unpack.repairRemnants(nil, []string{dir}, &xtractr.Response{Refused: []xtractr.RefusedFile{{Dest: file}}})

	if got != 1 {
		t.Fatalf("repairRemnants numeric: cleared %d, want 1", got)
	}

	if _, err := os.Stat(file + remnantSuffix + ".1"); err != nil {
		t.Fatalf("expected .remnant.1: %v", err)
	}
}

func TestHandleXtractrCallbackRestartsOnRemnant(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")

	if err := os.WriteFile(file, []byte("truncated"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	item := &Extract{
		Path:   dir,
		App:    starr.Sonarr,
		Status: EXTRACTING,
	}
	unpack.Map["item"] = item

	unpack.handleXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		X:       &xtractr.Xtract{Name: "item", Path: dir},
		Refused: []xtractr.RefusedFile{{Dest: file}},
	})

	if item.Status != WAITING {
		t.Fatalf("status = %s, want WAITING", item.Status.Desc())
	}

	if item.Retries != 1 {
		t.Fatalf("retries = %d, want 1", item.Retries)
	}

	if unpack.Retries != 1 {
		t.Fatalf("global retries = %d, want 1", unpack.Retries)
	}

	if _, err := os.Stat(file + remnantSuffix); err != nil {
		t.Fatalf("expected renamed remnant: %v", err)
	}
}

func TestHandleXtractrCallbackKeepsTorrentFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")

	if err := os.WriteFile(file, []byte("from torrent"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	item := &Extract{
		Path:     dir,
		App:      starr.Sonarr,
		Status:   EXTRACTING,
		PreFiles: snapshotDir(dir),
	}
	unpack.Map["item"] = item

	unpack.handleXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		X:       &xtractr.Xtract{Name: "item", Path: dir},
		Refused: []xtractr.RefusedFile{{Dest: file}},
	})

	if item.Status != EXTRACTED {
		t.Fatalf("status = %s, want EXTRACTED", item.Status.Desc())
	}

	if item.Retries != 0 {
		t.Fatalf("retries = %d, want 0", item.Retries)
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("torrent file should remain: %v", err)
	}
}

func TestHandleXtractrCallbackRetriesExhausted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")

	if err := os.WriteFile(file, []byte("truncated"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	unpack.MaxRetries = 3
	item := &Extract{
		Path:    dir,
		App:     starr.Sonarr,
		Status:  EXTRACTING,
		Retries: 3,
	}
	unpack.Map["item"] = item

	unpack.handleXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		X:       &xtractr.Xtract{Name: "item", Path: dir},
		Refused: []xtractr.RefusedFile{{Dest: file}},
	})

	if item.Status != EXTRACTFAILED {
		t.Fatalf("status = %s, want EXTRACTFAILED", item.Status.Desc())
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("dest should remain when retries are exhausted: %v", err)
	}
}

func TestFolderDestRoots(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	extract := t.TempDir()

	cfg := &FolderConfig{MoveBack: false, ExtractPath: extract}
	output := folderOutputPath(dir, cfg)
	wantPrefix := filepath.Join(extract, filepath.Base(dir)+suffix)

	if output != wantPrefix {
		t.Fatalf("folderOutputPath: got %s, want %s", output, wantPrefix)
	}

	roots := folderDestRoots(dir, cfg, "")
	foundOutput := false

	for _, root := range roots {
		if root == output {
			foundOutput = true
		}
	}

	if !foundOutput {
		t.Fatalf("folderDestRoots missing output %s in %v", output, roots)
	}

	archive := filepath.Join(dir, "movie.tar.gz")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	if got := folderMoveDest(archive); got != strings.TrimSuffix(archive, ".gz") {
		t.Fatalf("folderMoveDest archive: got %s", got)
	}

	final := folderTempFinalDest(archive, &FolderConfig{})
	// movie.tar.gz -> strip suffix from output (name+suffix) then two archive exts -> movie
	wantFinal := filepath.Join(dir, "movie")
	if final != wantFinal {
		t.Fatalf("folderTempFinalDest: got %s, want %s", final, wantFinal)
	}
}

func TestFolderXtractrCallbackClearsRemnant(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	output := dir + suffix

	if err := os.Mkdir(output, 0o750); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	file := filepath.Join(output, "movie.mkv")
	if err := os.WriteFile(file, []byte("truncated"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	unpack.folders = &Folders{
		Folders: map[string]*Folder{
			dir: {
				status: EXTRACTING,
				config: &FolderConfig{Path: dir},
			},
		},
	}
	unpack.Map[dir] = &Extract{Path: dir, App: FolderString, Status: EXTRACTING, IDs: map[string]any{"title": dir}}

	unpack.folderXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		Output:  output,
		X:       &xtractr.Xtract{Name: dir, Path: dir},
		Refused: []xtractr.RefusedFile{{Dest: file}},
	})

	folder := unpack.folders.Folders[dir]
	if folder.status != EXTRACTFAILED {
		t.Fatalf("folder status = %s, want EXTRACTFAILED", folder.status.Desc())
	}

	if unpack.Map[dir].Status != EXTRACTFAILED {
		t.Fatalf("map status = %s, want EXTRACTFAILED", unpack.Map[dir].Status.Desc())
	}

	if _, err := os.Stat(file + remnantSuffix); err != nil {
		t.Fatalf("expected renamed remnant: %v", err)
	}
}

func TestFolderXtractrCallbackRetriesExhausted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	output := dir + suffix

	if err := os.Mkdir(output, 0o750); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	file := filepath.Join(output, "movie.mkv")
	if err := os.WriteFile(file, []byte("truncated"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	unpack := New()
	unpack.MaxRetries = 3
	unpack.folders = &Folders{
		Folders: map[string]*Folder{
			dir: {
				status:  EXTRACTING,
				retries: 3,
				config:  &FolderConfig{Path: dir},
			},
		},
	}
	unpack.Map[dir] = &Extract{Path: dir, App: FolderString, Status: EXTRACTING, IDs: map[string]any{"title": dir}}

	unpack.folderXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		Output:  output,
		X:       &xtractr.Xtract{Name: dir, Path: dir},
		Refused: []xtractr.RefusedFile{{Dest: file}},
	})

	if unpack.folders.Folders[dir].status != EXTRACTFAILED {
		t.Fatalf("folder status = %s, want EXTRACTFAILED", unpack.folders.Folders[dir].status.Desc())
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("dest should remain when retries are exhausted: %v", err)
	}
}
