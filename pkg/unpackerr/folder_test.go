package unpackerr

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}
func (noopLogger) Errorf(string, ...any) {}
func (noopLogger) Debugf(string, ...any) {}

func TestNormalizeFolderExcludePaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	relative := "permanent"
	absolute := filepath.Join(base, "keep")

	paths := normalizeFolderExcludePaths(base, []string{"", "  ", relative, absolute})
	if len(paths) != 2 {
		t.Fatalf("expected 2 normalized paths, got %d: %v", len(paths), paths)
	}

	if paths[0] != filepath.Join(base, relative) {
		t.Fatalf("unexpected relative path normalization: %q", paths[0])
	}

	if paths[1] != absolute {
		t.Fatalf("unexpected absolute path normalization: %q", paths[1])
	}
}

func TestFolderConfigIsExcludedPath(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	excluded := filepath.Join(base, "permanent")
	cfg := &FolderConfig{ExcludePaths: StringSlice{excluded}}

	if !cfg.isExcludedPath(excluded) {
		t.Fatal("expected exact excluded path to match")
	}

	if !cfg.isExcludedPath(filepath.Join(excluded, "sub", "file.rar")) {
		t.Fatal("expected child path of excluded folder to match")
	}

	if cfg.isExcludedPath(excluded + "_other") {
		t.Fatal("did not expect prefix-only sibling path to match")
	}
}

func TestFoldersProcessEventCurrentBehavior(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	cfg := &FolderConfig{Path: watchPath}
	folders := newTestFolders(t, cfg)

	archive := filepath.Join(watchPath, "movie.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating archive test file: %v", err)
	}

	folders.processEvent(&eventData{
		cnfg: cfg,
		name: filepath.Base(archive),
		file: archive,
		op:   "test",
	}, time.Now())

	if _, ok := folders.Folders[archive]; !ok {
		t.Fatalf("expected archive path to be tracked: %s", archive)
	}

	plain := filepath.Join(watchPath, "note.txt")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating non-archive test file: %v", err)
	}

	folders.processEvent(&eventData{
		cnfg: cfg,
		name: filepath.Base(plain),
		file: plain,
		op:   "test",
	}, time.Now())

	if _, ok := folders.Folders[plain]; ok {
		t.Fatalf("did not expect non-archive file to be tracked: %s", plain)
	}

	dir := filepath.Join(watchPath, "incoming")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("creating folder test dir: %v", err)
	}

	folders.processEvent(&eventData{
		cnfg: cfg,
		name: filepath.Base(dir),
		file: dir,
		op:   "test",
	}, time.Now())

	if _, ok := folders.Folders[dir]; !ok {
		t.Fatalf("expected folder path to be tracked: %s", dir)
	}
}

func TestFoldersProcessEventExcludedPath(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	excluded := filepath.Join(watchPath, "permanent")
	if err := os.MkdirAll(filepath.Join(excluded, "sub"), 0o755); err != nil {
		t.Fatalf("creating excluded test path: %v", err)
	}

	nested := filepath.Join(excluded, "sub", "file.rar")
	if err := os.WriteFile(nested, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating nested archive file: %v", err)
	}

	cfg := &FolderConfig{
		Path:         watchPath,
		ExcludePaths: StringSlice{excluded},
	}
	folders := newTestFolders(t, cfg)

	// Direct excluded folder.
	folders.processEvent(&eventData{
		cnfg: cfg,
		name: "permanent",
		file: excluded,
		op:   "test",
	}, time.Now())

	if len(folders.Folders) != 0 {
		t.Fatalf("expected no tracked folders for excluded path, got: %v", folders.Folders)
	}

	// Nested event from an excluded folder should also be ignored.
	folders.processEvent(&eventData{
		cnfg: cfg,
		name: "sub",
		file: nested,
		op:   "test",
	}, time.Now())

	if len(folders.Folders) != 0 {
		t.Fatalf("expected no tracked folders for nested excluded event, got: %v", folders.Folders)
	}
}

func TestFoldersHandleFileEventExcludedPath(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	excluded := filepath.Join(watchPath, "permanent")

	folders := &Folders{
		Config: []*FolderConfig{{
			Path:         watchPath,
			ExcludePaths: StringSlice{excluded},
		}},
		Events: make(chan *eventData, 1),
		Logs:   noopLogger{},
	}

	folders.handleFileEvent(filepath.Join(excluded, "file.rar"), "test")

	select {
	case event := <-folders.Events:
		t.Fatalf("did not expect event for excluded path: %+v", event)
	default:
	}
}

func newTestFolders(t *testing.T, cfg *FolderConfig) *Folders {
	t.Helper()

	folders, err := (FoldersConfig{Buffer: 32}).newWatcher([]*FolderConfig{cfg}, noopLogger{})
	if err != nil {
		t.Fatalf("creating watcher: %v", err)
	}

	t.Cleanup(func() {
		if folders.Watcher != nil {
			folders.Watcher.Close()
		}
		if folders.FSNotify != nil {
			folders.FSNotify.Close()
		}
	})

	return folders
}
