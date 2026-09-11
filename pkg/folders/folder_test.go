package folders

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

func TestNormalizeExcludePaths(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	relative := "permanent"
	absolute := filepath.Join(base, "keep")

	paths := NormalizeExcludePaths(base, []string{"", "  ", relative, absolute})
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
	cfg := &FolderConfig{ExcludePaths: []string{excluded}}

	if !cfg.IsExcludedPath(excluded) {
		t.Fatal("expected exact excluded path to match")
	}

	if !cfg.IsExcludedPath(filepath.Join(excluded, "sub", "file.rar")) {
		t.Fatal("expected child path of excluded folder to match")
	}

	if cfg.IsExcludedPath(excluded + "_other") {
		t.Fatal("did not expect prefix-only sibling path to match")
	}
}

func TestFoldersProcessEventCurrentBehavior(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	cfg := &FolderConfig{Path: watchPath}
	tracker := newTestFolders(t, cfg)

	archive := filepath.Join(watchPath, "movie.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating archive test file: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   filepath.Base(archive),
		File:   archive,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[archive]; !ok {
		t.Fatalf("expected archive path to be tracked: %s", archive)
	}

	plain := filepath.Join(watchPath, "note.txt")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating non-archive test file: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   filepath.Base(plain),
		File:   plain,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[plain]; ok {
		t.Fatalf("did not expect non-archive file to be tracked: %s", plain)
	}

	dir := filepath.Join(watchPath, "incoming")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("creating folder test dir: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   filepath.Base(dir),
		File:   dir,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[dir]; !ok {
		t.Fatalf("expected folder path to be tracked: %s", dir)
	}
}

func TestFoldersProcessEventExcludedPath(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()

	excluded := filepath.Join(watchPath, "permanent")
	if err := os.MkdirAll(filepath.Join(excluded, "sub"), 0o700); err != nil {
		t.Fatalf("creating excluded test path: %v", err)
	}

	nested := filepath.Join(excluded, "sub", "file.rar")
	if err := os.WriteFile(nested, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating nested archive file: %v", err)
	}

	cfg := &FolderConfig{
		Path:         watchPath,
		ExcludePaths: []string{excluded},
	}
	tracker := newTestFolders(t, cfg)

	// Direct excluded folder.
	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   "permanent",
		File:   excluded,
		Op:     "test",
	}, time.Now())

	if len(tracker.Folders) != 0 {
		t.Fatalf("expected no tracked folders for excluded path, got: %v", tracker.Folders)
	}

	// Nested event from an excluded folder should also be ignored.
	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   "sub",
		File:   nested,
		Op:     "test",
	}, time.Now())

	if len(tracker.Folders) != 0 {
		t.Fatalf("expected no tracked folders for nested excluded event, got: %v", tracker.Folders)
	}
}

func TestFoldersHandleFileEventExcludedPath(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	excluded := filepath.Join(watchPath, "permanent")

	tracker := &Folders{
		Config: []*FolderConfig{{
			Path:         watchPath,
			ExcludePaths: []string{excluded},
		}},
		Events: make(chan *Event, 1),
		Logs:   noopLogger{},
	}

	tracker.handleFileEvent(filepath.Join(excluded, "file.rar"), "test")

	select {
	case event := <-tracker.Events:
		t.Fatalf("did not expect event for excluded path: %+v", event)
	default:
	}
}

func newTestFolders(t *testing.T, cfg *FolderConfig) *Folders {
	t.Helper()

	tracker, err := (WatchConfig{Buffer: 32}).NewWatcher([]*FolderConfig{cfg}, noopLogger{}, 1, "")
	if err != nil {
		t.Fatalf("creating watcher: %v", err)
	}

	t.Cleanup(func() {
		if tracker.Watcher != nil {
			tracker.Watcher.Close()
		}

		if tracker.FSNotify != nil {
			_ = tracker.FSNotify.Close()
		}
	})

	return tracker
}
