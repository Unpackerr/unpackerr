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

func TestExtractLimitsDefaults(t *testing.T) {
	t.Parallel()

	unpack := New()

	got, err := unpack.extractLimits()
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}

	wantBytes, err := parseExtractMaxBytes(defaultMaxBytes)
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}

	if got.bytes != wantBytes {
		t.Fatalf("defaults: got %+v", got)
	}
}

func TestExtractLimitsUnlimited(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.MaxBytes = "0"

	got, err := unpack.extractLimits()
	if err != nil {
		t.Fatalf("unlimited: %v", err)
	}

	if got.bytes != 0 {
		t.Fatalf("0 must be unlimited, got %+v", got)
	}
}

func TestValidateFoldersExtrasDefaults(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Folders = []*FolderConfig{
		{Path: "unset"},
		{Path: "custom", MaxNested: 32, ExtrasMaxDepth: 6, AllowSymlinks: true},
		{Path: "unlimited", MaxNested: -1, ExtrasMaxDepth: -1},
	}

	if err := unpack.validateFolders(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	if unpack.Folders[0].MaxNested != defaultMaxNested ||
		unpack.Folders[0].ExtrasMaxDepth != defaultExtrasMaxDepth ||
		unpack.Folders[0].MaxFiles != defaultMaxFiles ||
		unpack.Folders[0].MaxRatio != defaultMaxRatio {
		t.Fatalf("unset defaults: %+v", unpack.Folders[0])
	}

	if unpack.Folders[1].MaxNested != 32 || unpack.Folders[1].ExtrasMaxDepth != 6 || !unpack.Folders[1].AllowSymlinks {
		t.Fatalf("custom: %+v", unpack.Folders[1])
	}

	if unpack.Folders[2].MaxNested != -1 || unpack.Folders[2].ExtrasMaxDepth != -1 {
		t.Fatalf("unlimited: %+v", unpack.Folders[2])
	}
}

func TestResolvedMaxBytes(t *testing.T) {
	t.Parallel()

	if got := resolvedMaxBytes(false, 99, 75); got != 75 {
		t.Fatalf("unset override should use global, got %d", got)
	}

	if got := resolvedMaxBytes(true, 0, 75); got != 0 {
		t.Fatalf("explicit 0 is unlimited, got %d", got)
	}

	if got := resolvedMaxBytes(true, 10, 75); got != 10 {
		t.Fatalf("override, got %d", got)
	}
}

func TestParseExtractMaxBytes(t *testing.T) {
	t.Parallel()

	n, err := parseExtractMaxBytes(defaultMaxBytes)
	if err != nil || n != 75*1024*1024*1024 {
		t.Fatalf("%s: n=%d err=%v", defaultMaxBytes, n, err)
	}

	if _, err := parseExtractMaxBytes("nope"); err == nil {
		t.Fatal("invalid max_bytes must error")
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
