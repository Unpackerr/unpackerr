package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
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

func TestApplyMaxBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		app  starr.App
		size string
	}{
		{starr.Sonarr, defaultSonarrMaxBytes},
		{starr.Radarr, defaultRadarrMaxBytes},
		{starr.Lidarr, defaultLidarrMaxBytes},
		{starr.Readarr, defaultReadarrMaxBytes},
		{starr.Whisparr, defaultWhisparrMaxBytes},
	}

	for _, testCase := range cases {
		conf := &StarrConfig{}
		if err := conf.applyMaxBytes(testCase.app); err != nil {
			t.Fatalf("%s default: %v", testCase.app, err)
		}

		want, err := parseExtractMaxBytes(testCase.size)
		if err != nil {
			t.Fatalf("parse %s: %v", testCase.size, err)
		}

		if conf.maxBytes != want {
			t.Fatalf("%s: got %d want %d", testCase.app, conf.maxBytes, want)
		}
	}

	unlimited := &StarrConfig{MaxBytes: "0"}
	if err := unlimited.applyMaxBytes(starr.Radarr); err != nil {
		t.Fatalf("unlimited: %v", err)
	}

	if unlimited.maxBytes != 0 {
		t.Fatalf("0 must be unlimited, got %d", unlimited.maxBytes)
	}

	custom := &StarrConfig{MaxBytes: "10GB"}
	if err := custom.applyMaxBytes(starr.Sonarr); err != nil {
		t.Fatalf("custom: %v", err)
	}

	want, err := parseExtractMaxBytes("10GB")
	if err != nil {
		t.Fatalf("parse 10GB: %v", err)
	}

	if custom.maxBytes != want {
		t.Fatalf("custom override: got %d want %d", custom.maxBytes, want)
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

	if unpack.Folders[0].MaxNested != 0 ||
		unpack.Folders[0].ExtrasMaxDepth != 0 ||
		unpack.Folders[0].MaxFiles != 0 ||
		unpack.Folders[0].MaxRatio != 0 ||
		unpack.Folders[0].maxBytes != 0 {
		t.Fatalf("unset must stay unlimited: %+v", unpack.Folders[0])
	}

	if unpack.Folders[1].MaxNested != 32 || unpack.Folders[1].ExtrasMaxDepth != 6 || !unpack.Folders[1].AllowSymlinks {
		t.Fatalf("custom: %+v", unpack.Folders[1])
	}

	if unpack.Folders[2].MaxNested != -1 || unpack.Folders[2].ExtrasMaxDepth != -1 {
		t.Fatalf("unlimited: %+v", unpack.Folders[2])
	}
}

func TestMaxRetriesZeroUsesDefault(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.MaxRetries = 0

	if unpack.maxRetries() != defaultMaxRetries {
		t.Fatalf("0 must use default %d, got %d", defaultMaxRetries, unpack.maxRetries())
	}

	unpack.MaxRetries = 7

	if unpack.maxRetries() != 7 {
		t.Fatalf("explicit override: got %d", unpack.maxRetries())
	}
}

func TestHandleXtractrCallbackLimitNoRetry(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Map["bomb"] = &Extract{App: starr.Sonarr, Path: t.TempDir()}
	unpack.handleXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		X:       &xtractr.Xtract{Name: "bomb"},
		Error:   xtractr.ErrMaxBytes,
	})

	item := unpack.Map["bomb"]
	if item.Status != EXTRACTFAILED || !item.NoRetry {
		t.Fatalf("limit error must stay failed without retry: %+v", item)
	}

	unpack.Map["io"] = &Extract{App: starr.Sonarr, Path: t.TempDir()}
	unpack.handleXtractrCallback(&xtractr.Response{
		Done:    true,
		Started: time.Now(),
		X:       &xtractr.Xtract{Name: "io"},
		Error:   os.ErrPermission,
	})

	if unpack.Map["io"].NoRetry {
		t.Fatal("non-limit errors must still retry")
	}
}

func TestCheckExtractDoneExhaustedStaysFailed(t *testing.T) {
	t.Parallel()

	unpack := New()
	now := time.Now()
	unpack.Map["stuck"] = &Extract{
		App:     starr.Sonarr,
		Status:  EXTRACTFAILED,
		Retries: unpack.maxRetries(),
		Updated: now.Add(-time.Hour),
	}

	unpack.checkExtractDone(now)

	item := unpack.Map["stuck"]
	if item == nil || item.Status != EXTRACTFAILED || !item.NoRetry {
		t.Fatalf("exhausted retries must stay EXTRACTFAILED: %+v", item)
	}

	unpack.checkExtractDone(now.Add(time.Hour))

	if unpack.Map["stuck"].Status != EXTRACTFAILED {
		t.Fatalf("later tick must not bounce to %s", unpack.Map["stuck"].Status.Desc())
	}
}

func TestParseExtractMaxBytes(t *testing.T) {
	t.Parallel()

	n, err := parseExtractMaxBytes(defaultRadarrMaxBytes)
	if err != nil || n != 75*1024*1024*1024 {
		t.Fatalf("%s: n=%d err=%v", defaultRadarrMaxBytes, n, err)
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
