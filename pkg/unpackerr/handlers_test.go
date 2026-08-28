package unpackerr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

func TestSliceToSet(t *testing.T) {
	t.Parallel()

	got := sliceToSet([]string{"a", "b", "a"})
	if len(got) != 2 {
		t.Fatalf("sliceToSet: expected 2 keys, got %d", len(got))
	}

	if _, ok := got["a"]; !ok {
		t.Fatal("sliceToSet: missing a")
	}

	if _, ok := got["b"]; !ok {
		t.Fatal("sliceToSet: missing b")
	}
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

	pre := sliceToSet([]string{filepath.Base(keep)})
	got := unpack.repairRemnants(pre, dir, resp)

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

	got := unpack.repairRemnants(nil, dir, resp)
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

	got := unpack.repairRemnants(nil, dir, resp)
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
	got := unpack.repairRemnants(nil, dir, &xtractr.Response{Refused: []xtractr.RefusedFile{{Dest: file}}})

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
		PreFiles: sliceToSet([]string{"movie.mkv"}),
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

	if item.Status != EXTRACTED {
		t.Fatalf("status = %s, want EXTRACTED", item.Status.Desc())
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("dest should remain when retries are exhausted: %v", err)
	}
}

func TestFolderMoveDest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if got := folderMoveDest(dir); got != dir {
		t.Fatalf("dir dest: got %s, want %s", got, dir)
	}

	archive := filepath.Join(dir, "movie.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	want := filepath.Join(dir, "movie")
	if got := folderMoveDest(archive); got != want {
		t.Fatalf("archive dest: got %s, want %s", got, want)
	}
}

func TestFolderXtractrCallbackClearsRemnant(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "movie.mkv")

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
