package unpackerr

import (
	"os"
	"path/filepath"
	"testing"

	"golift.io/xtractr"
)

func TestDirIsEmpty(t *testing.T) {
	t.Parallel()

	emptyDir := t.TempDir()
	if !dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return true on an empty folder")
	}

	f, err := os.Create(filepath.Join(emptyDir, "emptyFile"))
	if err != nil {
		t.Fatalf("Got an error making temp file: %v", err)
	}
	defer f.Close()

	if dirIsEmpty(emptyDir) {
		t.Fatal("dirIsEmpty should return false when the folder has a file in it")
	}
}

// newTestUnpackerrForPurge returns an Unpackerr with Xtractr set so purgeEmptyFolders can call DeleteFiles.
func newTestUnpackerrForPurge(t *testing.T) *Unpackerr {
	t.Helper()

	unpack := New()
	unpack.Xtractr = xtractr.NewQueue(&xtractr.Config{
		Parallel: 1,
		Suffix:   "_unpackerred",
		Logger:   unpack.Logger,
		FileMode: 0o644,
		DirMode:  0o755,
	})

	return unpack
}

func TestPurgeEmptyFoldersDedupe(t *testing.T) {
	t.Parallel()

	unpack := newTestUnpackerrForPurge(t)
	base := t.TempDir()
	subdir := filepath.Join(base, "subdir")

	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Multiple paths in the same folder: purge should consider subdir once and return 2 (subdir + base).
	paths := []string{
		filepath.Join(subdir, "f1.mkv"),
		filepath.Join(subdir, "f2.mkv"),
		filepath.Join(subdir, "f3.mkv"),
	}

	purged := unpack.purgeEmptyFolders(paths, base)

	if purged != 2 {
		t.Fatalf("purgeEmptyFolders: expected 2 purged (subdir + base), got %d", purged)
	}

	if _, err := os.Stat(subdir); err == nil {
		t.Fatal("subdir should have been removed")
	}

	if _, err := os.Stat(base); err == nil {
		t.Fatal("base should have been removed")
	}
}

func TestPurgeEmptyFoldersStopsAtRoot(t *testing.T) {
	t.Parallel()

	unpack := newTestUnpackerrForPurge(t)
	base := t.TempDir()
	root := filepath.Join(base, "download")
	subdir := filepath.Join(root, "subdir")

	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths := []string{filepath.Join(subdir, "file.mkv")}

	purged := unpack.purgeEmptyFolders(paths, root)

	if purged != 2 {
		t.Fatalf("purgeEmptyFolders: expected 2 purged (subdir + root), got %d", purged)
	}

	if _, err := os.Stat(root); err == nil {
		t.Fatal("root (download) should have been removed")
	}

	// base should still exist (we never purge above root).
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("base should still exist: %v", err)
	}
}

func TestPurgeEmptyFoldersDoesNotPurgeNonEmpty(t *testing.T) {
	t.Parallel()

	unpack := newTestUnpackerrForPurge(t)
	base := t.TempDir()
	subdir := filepath.Join(base, "subdir")

	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Keep base non-empty so it is not purged.
	keep := filepath.Join(base, "keep.txt")

	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	paths := []string{filepath.Join(subdir, "file.mkv")}

	purged := unpack.purgeEmptyFolders(paths, base)

	if purged != 1 {
		t.Fatalf("purgeEmptyFolders: expected 1 purged (subdir only), got %d", purged)
	}

	if _, err := os.Stat(subdir); err == nil {
		t.Fatal("subdir should have been removed")
	}

	if _, err := os.Stat(base); err != nil {
		t.Fatalf("base should still exist: %v", err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("keep file should still exist: %v", err)
	}
}

func TestPurgeEmptyFoldersNoRoot(t *testing.T) {
	t.Parallel()

	unpack := newTestUnpackerrForPurge(t)
	base := t.TempDir()
	subdir := filepath.Join(base, "subdir")

	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths := []string{filepath.Join(subdir, "file.mkv")}

	purged := unpack.purgeEmptyFolders(paths, "")

	// Without root we purge all empty ancestors; at least subdir and base.
	if purged < 2 {
		t.Fatalf("purgeEmptyFolders: expected at least 2 purged (subdir + base), got %d", purged)
	}

	if _, err := os.Stat(subdir); err == nil {
		t.Fatal("subdir should have been removed")
	}

	if _, err := os.Stat(base); err == nil {
		t.Fatal("base should have been removed")
	}
}

func TestPurgeEmptyFoldersEmptyPaths(t *testing.T) {
	t.Parallel()

	unpack := newTestUnpackerrForPurge(t)

	purged := unpack.purgeEmptyFolders(nil, "")

	if purged != 0 {
		t.Fatalf("purgeEmptyFolders(nil): expected 0, got %d", purged)
	}

	purged = unpack.purgeEmptyFolders([]string{}, "")

	if purged != 0 {
		t.Fatalf("purgeEmptyFolders([]): expected 0, got %d", purged)
	}
}
