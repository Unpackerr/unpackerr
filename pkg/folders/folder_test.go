package folders

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
)

type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}
func (noopLogger) Errorf(string, ...any) {}
func (noopLogger) Debugf(string, ...any) {}

type captureLogger struct {
	errs []string
}

func (c *captureLogger) Printf(string, ...any) {}
func (c *captureLogger) Debugf(string, ...any) {}
func (c *captureLogger) Errorf(msg string, v ...any) {
	c.errs = append(c.errs, fmt.Sprintf(msg, v...))
}

func TestValidateListRequiresPath(t *testing.T) {
	t.Parallel()

	err := ValidateList([]*FolderConfig{{DeleteOrig: true}}, func(string) (uint64, bool, error) {
		return 0, false, nil
	})
	if !errors.Is(err, ErrNoPath) {
		t.Fatalf("empty path: %v", err)
	}
}

func TestCheckSkipsEmptyPath(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	log := &captureLogger{}

	good, names := Check([]*FolderConfig{{Path: "", DeleteOrig: true}}, log)
	if len(good) != 0 || len(names) != 0 {
		t.Fatalf("watched empty path: %v %v", names, good)
	}

	for _, name := range names {
		if name == cwd {
			t.Fatal("empty path must not watch the working directory")
		}
	}

	if len(log.errs) == 0 || !strings.Contains(log.errs[0], "empty path") {
		t.Fatalf("log %v", log.errs)
	}
}

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

func TestFoldersIgnoreExtractDestLogFile(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	cfg := &FolderConfig{Path: watchPath}
	tracker := newTestFolders(t, cfg)
	tracker.IgnoreSuffix = "_unpackerred"

	dest := filepath.Join(watchPath, "Win10")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatalf("creating extract dest: %v", err)
	}

	logName := filepath.Join(dest, "_unpackerred.Win10.iso.txt")
	if err := os.WriteFile(logName, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating extract log: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   "Win10",
		File:   dest,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[dest]; ok {
		t.Fatalf("did not expect extract dest to be tracked: %s", dest)
	}
}

func TestFoldersIgnoreExtractDestSiblingArchive(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	cfg := &FolderConfig{Path: watchPath}
	tracker := newTestFolders(t, cfg)
	tracker.IgnoreSuffix = "_unpackerred"

	iso := filepath.Join(watchPath, "Win10.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating sibling iso: %v", err)
	}

	dest := filepath.Join(watchPath, "Win10")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatalf("creating extract dest: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   "Win10",
		File:   dest,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[dest]; ok {
		t.Fatalf("did not expect sibling-of-archive dest to be tracked: %s", dest)
	}

	incoming := filepath.Join(watchPath, "incoming")
	if err := os.Mkdir(incoming, 0o700); err != nil {
		t.Fatalf("creating incoming dir: %v", err)
	}

	tracker.ProcessEvent(&Event{
		Config: cfg,
		Name:   "incoming",
		File:   incoming,
		Op:     "test",
	}, time.Now())

	if _, ok := tracker.Folders[incoming]; !ok {
		t.Fatalf("expected unrelated folder to be tracked: %s", incoming)
	}
}

func TestFoldersUntrackExtractDestAfterSiblingAppears(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	cfg := &FolderConfig{Path: watchPath}
	tracker := newTestFolders(t, cfg)
	tracker.IgnoreSuffix = "_unpackerred"

	dest := filepath.Join(watchPath, "Win10")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatalf("creating dest: %v", err)
	}

	now := time.Now()
	tracker.ProcessEvent(&Event{Config: cfg, Name: "Win10", File: dest, Op: "test"}, now)

	if _, ok := tracker.Folders[dest]; !ok {
		t.Fatalf("expected dest to be tracked before sibling archive: %s", dest)
	}

	iso := filepath.Join(watchPath, "Win10.iso")
	if err := os.WriteFile(iso, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating sibling iso: %v", err)
	}

	tracker.ProcessEvent(&Event{Config: cfg, Name: "Win10", File: dest, Op: "write"}, now.Add(time.Second))

	if _, ok := tracker.Folders[dest]; ok {
		t.Fatalf("expected dest to be untracked after sibling archive appeared: %s", dest)
	}
}

func TestFoldersHandleFileEventIgnoreExtractSuffixNested(t *testing.T) {
	t.Parallel()

	watchPath := t.TempDir()
	tracker := &Folders{
		Config:       []*FolderConfig{{Path: watchPath}},
		Events:       make(chan *Event, 1),
		Logs:         noopLogger{},
		IgnoreSuffix: "_unpackerred",
	}

	nested := filepath.Join(watchPath, "Win10.iso_unpackerred", "autorun.inf")
	tracker.handleFileEvent(nested, "test")

	select {
	case event := <-tracker.Events:
		t.Fatalf("did not expect event for nested extract path: %+v", event)
	default:
	}
}

func TestArchiveStem(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"Win10.iso":    "Win10",
		"movie.tar.gz": "movie",
		"show.7z":      "show",
		"plain":        "plain",
		"archive.ZIP":  "archive",
	}

	for name, want := range tests {
		if got := archiveStem(name); got != want {
			t.Fatalf("archiveStem(%q)=%q want %q", name, got, want)
		}
	}
}

func newTestFolders(t *testing.T, cfg *FolderConfig) *Folders {
	t.Helper()

	tracker, err := (WatchConfig{Buffer: 32}).NewWatcher([]*FolderConfig{cfg}, noopLogger{}, 1, "")
	if err != nil {
		t.Fatalf("creating watcher: %v", err)
	}

	t.Cleanup(tracker.Close)

	return tracker
}

func TestUsesPoller(t *testing.T) {
	t.Parallel()

	if (&FolderConfig{}).UsesPoller() {
		t.Fatal("zero interval must be off")
	}

	if (&FolderConfig{Interval: cnfg.Duration{Duration: time.Millisecond}}).UsesPoller() {
		t.Fatal("1ms is below MinimumPollInterval")
	}

	if !(&FolderConfig{Interval: cnfg.Duration{Duration: time.Second}}).UsesPoller() {
		t.Fatal("1s must enable the poller")
	}
}

func TestPollerWatchesRootNotExistingTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	deep := filepath.Join(nested, "inside.rar")
	if err := os.WriteFile(deep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: root, Interval: cnfg.Duration{Duration: time.Second}}

	tracker, err := (WatchConfig{Buffer: 8}).NewWatcher([]*FolderConfig{cfg}, noopLogger{}, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(tracker.Close)

	if tracker.FSNotify != nil {
		t.Fatal("poller-only folder must not open fsnotify")
	}

	if len(tracker.pollers) != 1 {
		t.Fatalf("pollers %d", len(tracker.pollers))
	}

	watched := tracker.pollers[0].watcher.WatchedFiles()
	if _, ok := watched[deep]; ok {
		t.Fatal("must not recurse into existing subfolders at start")
	}

	if err := tracker.Add(nested); err != nil {
		t.Fatal(err)
	}

	if _, ok := tracker.pollers[0].watcher.WatchedFiles()[deep]; !ok {
		t.Fatal("nested Add should watch inside a folder that appears after start")
	}

	tracker.Remove(nested)

	if _, ok := tracker.pollers[0].watcher.WatchedFiles()[deep]; ok {
		t.Fatal("Remove should drop the nested poller watch")
	}
}

func TestFSNotifyFolderHasNoPoller(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	nested := filepath.Join(root, "sub")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := &FolderConfig{Path: root}
	tracker := newTestFolders(t, cfg)

	if len(tracker.pollers) != 0 {
		t.Fatalf("pollers %d", len(tracker.pollers))
	}

	if tracker.FSNotify == nil {
		t.Fatal("expected fsnotify")
	}

	if err := tracker.Add(nested); err != nil {
		t.Fatal(err)
	}
}

func TestMixedPollerAndFSNotify(t *testing.T) {
	t.Parallel()

	pollPath := t.TempDir()
	fsPath := t.TempDir()
	cfgs := []*FolderConfig{
		{Path: pollPath, Interval: cnfg.Duration{Duration: time.Second}},
		{Path: fsPath},
	}

	tracker, err := (WatchConfig{Buffer: 8}).NewWatcher(cfgs, noopLogger{}, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(tracker.Close)

	if len(tracker.pollers) != 1 {
		t.Fatalf("pollers %d", len(tracker.pollers))
	}

	if got := tracker.PollerSummaries(); len(got) != 1 || !strings.Contains(got[0], pollPath) {
		t.Fatalf("poller summaries %v", got)
	}

	if got := tracker.FSNotifyPaths(); len(got) != 1 || got[0] != fsPath {
		t.Fatalf("fsnotify paths %v", got)
	}
}

func TestPathContainsRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Clean("/")
	item := filepath.Join(root, "downloads", "movie.rar")

	if !PathContains(root, item) {
		t.Fatalf("%q should contain %q", root, item)
	}

	if PathContains(filepath.Join(root, "data"), filepath.Join(root, "data-old", "a.rar")) {
		t.Fatal("sibling prefix must not match")
	}
}

func TestPollerForRootWatch(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Clean("/")
	tracker := &Folders{
		Config:  []*FolderConfig{{Path: rootPath}},
		pollers: []*folderPoller{{path: rootPath}},
	}

	if got := tracker.pollerFor(filepath.Join(rootPath, "new-dir")); got == nil || got.path != rootPath {
		t.Fatalf("root poller missed nested path: %+v", got)
	}
}

func TestHandleFileEventPrefersNestedWatch(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	child := filepath.Join(parent, "tv")

	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(child, "show.rar")
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	parentCfg := &FolderConfig{Path: parent}
	childCfg := &FolderConfig{Path: child}
	tracker := &Folders{
		Config: []*FolderConfig{parentCfg, childCfg},
		Events: make(chan *Event, 1),
		Logs:   noopLogger{},
	}

	tracker.handleFileEvent(archive, "test")

	select {
	case event := <-tracker.Events:
		if event.Config != childCfg {
			t.Fatalf("got watch %q want %q", event.Config.Path, child)
		}

		if event.Name != "show.rar" {
			t.Fatalf("name %q", event.Name)
		}
	default:
		t.Fatal("expected nested-watch event")
	}

	tracker.handleFileEvent(child, "test")

	select {
	case event := <-tracker.Events:
		t.Fatalf("did not expect event for nested watch root: %+v", event)
	default:
	}
}
