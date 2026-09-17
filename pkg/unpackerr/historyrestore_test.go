package unpackerr

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

func restoreTestUnpackerr(t *testing.T) *Unpackerr {
	t.Helper()

	unpack := New()
	unpack.KeepHistory = 20
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.RetryDelay.Duration = time.Minute
	unpack.Sonarr = instanceMap([]*SonarrConfig{{}})
	unpack.Sonarr["0"].Name = "Sportarr"
	unpack.Sonarr["0"].URL = "http://sonarr:8989"

	return unpack
}

func TestRestoreQueueExtractedAndImported(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	unpack.upsertHistory(HistoryRecord{
		ID: "show", Kind: string(starr.Sonarr), App: "Sportarr", URL: "http://sonarr:8989",
		Path: "/dl/show", Status: EXTRACTED, Updated: now.Add(-time.Hour),
		NewFiles: []string{"/dl/show/ep.mkv"}, DeleteDelay: "5m",
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "movie", Kind: string(starr.Radarr), App: "Radarr", URL: "http://radarr:7878",
		Path: "/dl/movie", Status: IMPORTED, Updated: now.Add(-time.Minute),
		NewFiles: []string{"/dl/movie/film.mkv"}, DeleteOrig: true, DeleteDelay: "2m",
	})
	unpack.restoreQueueFromHistory()

	show := unpack.Map["show"]
	if show == nil || show.Status != EXTRACTED || show.App != starr.Sonarr || show.Name != "Sportarr" {
		t.Fatalf("extracted %+v", show)
	}

	if show.Resp == nil || len(show.Resp.NewFiles) != 1 || show.DeleteDelay != 5*time.Minute {
		t.Fatalf("extracted files/delay %+v", show)
	}

	movie := unpack.Map["movie"]
	if movie == nil || movie.Status != IMPORTED || !movie.DeleteOrig || movie.App != starr.Radarr {
		t.Fatalf("imported %+v", movie)
	}
}

func TestRestoreQueueInterruptedAndQueued(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	unpack.upsertHistory(HistoryRecord{
		ID: "mid", Kind: string(starr.Sonarr), App: "Sportarr", URL: "http://sonarr:8989",
		Path: "/dl/mid", Status: EXTRACTING, Updated: now, PreFiles: []string{"/dl/mid/keep.txt"},
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "queued", Kind: string(starr.Lidarr), App: "Lidarr", Path: "/dl/album",
		Status: QUEUED, Updated: now,
	})
	unpack.restoreQueueFromHistory()

	mid := unpack.Map["mid"]
	if mid == nil || mid.Status != EXTRACTFAILED {
		t.Fatalf("extracting %+v", mid)
	}

	if _, ok := mid.PreFiles["/dl/mid/keep.txt"]; !ok {
		t.Fatalf("preFiles %+v", mid.PreFiles)
	}

	if mid.Resp == nil || !errors.Is(mid.Resp.Error, errInterruptedRestart) {
		t.Fatalf("interrupted %v", mid.Resp)
	}

	queued := unpack.Map["queued"]
	if queued == nil || queued.Status != WAITING || queued.App != starr.Lidarr {
		t.Fatalf("queued %+v", queued)
	}
}

func TestRestoreQueueSkipsOldAndDeleted(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	unpack.upsertHistory(HistoryRecord{
		ID: "old", Kind: string(starr.Sonarr), Path: "/dl/old", Status: IMPORTED,
		Updated: now.Add(-80 * time.Hour),
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "gone", Kind: string(starr.Sonarr), Path: "/dl/gone", Status: DELETED, Updated: now,
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map["old"] != nil || unpack.Map["gone"] != nil {
		t.Fatalf("skipped rows present: %+v", unpack.Map)
	}
}

func TestHistorySnapshotHidesInFlight(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	unpack.upsertHistory(HistoryRecord{
		ID: "show", Kind: string(starr.Sonarr), Path: "/dl/show",
		Status: EXTRACTED, Updated: now,
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "mid", Kind: string(starr.Sonarr), Path: "/dl/mid",
		Status: EXTRACTING, Updated: now,
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "queued", Kind: string(starr.Lidarr), Path: "/dl/album",
		Status: QUEUED, Updated: now,
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "done", Kind: string(starr.Radarr), Path: "/dl/movie",
		Status: IMPORTED, Updated: now,
	})

	for _, rec := range unpack.historySnapshot() {
		if rec.Status == EXTRACTED || rec.Status == EXTRACTING || rec.Status == QUEUED {
			t.Fatalf("history API leaked in-flight %+v", rec)
		}
	}

	if len(unpack.historySnapshot()) != 1 {
		t.Fatalf("want imported only, got %+v", unpack.historySnapshot())
	}
}

func TestRestoreQueueMatchesURLWhenKindMissing(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.Sonarr = instanceMap([]*SonarrConfig{{}})
	unpack.Sonarr["0"].URL = "http://sonarr:8989"

	unpack.upsertHistory(HistoryRecord{
		ID: "legacy", App: "Sportarr", URL: "http://sonarr:8989", Path: "/dl/legacy",
		Status: EXTRACTED, Updated: time.Now(),
	})

	unpack.restoreQueueFromHistory()

	got := unpack.Map["legacy"]
	if got == nil || got.App != starr.Sonarr || got.Name != "Sportarr" {
		t.Fatalf("%+v", got)
	}
}

func TestRestoreQueueSkipsExistingMapEntry(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.Map["show"] = &Extract{Path: "/live", Status: WAITING, App: starr.Sonarr}

	unpack.upsertHistory(HistoryRecord{
		ID: "show", Kind: string(starr.Sonarr), Path: "/dl/show",
		Status: EXTRACTED, Updated: time.Now(),
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map["show"].Path != "/live" || unpack.Map["show"].Status != WAITING {
		t.Fatalf("clobbered %+v", unpack.Map["show"])
	}
}

func TestMaybeRecordHistoryWritesExtracted(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	unpack.maybeRecordHistory("show", &Extract{
		App:     starr.Sonarr,
		Name:    "Sportarr",
		Path:    "/dl/show",
		Status:  EXTRACTED,
		Updated: time.Now(),
		Resp:    &xtractr.Response{NewFiles: []string{"/dl/show/ep.mkv"}},
	})

	if len(unpack.records) != 1 || unpack.records[0].Kind != string(starr.Sonarr) ||
		len(unpack.records[0].NewFiles) != 1 {
		t.Fatalf("%+v", unpack.records)
	}

	if len(unpack.historySnapshot()) != 0 {
		t.Fatal("extracted should not appear on GET /api/history")
	}
}

func TestClearHistoryKeepsExtracted(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	unpack.upsertHistory(HistoryRecord{
		ID: "show", Kind: string(starr.Sonarr), App: "Sportarr", URL: "http://sonarr:8989",
		Path: "/dl/show", Status: EXTRACTED, Updated: now, NewFiles: []string{"/dl/show/ep.mkv"},
	})
	unpack.upsertHistory(HistoryRecord{
		ID: "movie", Kind: string(starr.Radarr), Path: "/dl/movie", Status: IMPORTED, Updated: now,
	})

	if err := unpack.clearHistory(); err != nil {
		t.Fatal(err)
	}

	if len(unpack.historySnapshot()) != 0 {
		t.Fatalf("finished rows after clear %+v", unpack.historySnapshot())
	}

	if len(unpack.records) != 1 || unpack.records[0].ID != "show" {
		t.Fatalf("resume row %+v", unpack.records)
	}

	unpack.restoreQueueFromHistory()

	show := unpack.Map["show"]
	if show == nil || show.Status != EXTRACTED {
		t.Fatalf("extracted after clear %+v", show)
	}

	if unpack.Map["movie"] != nil {
		t.Fatal("cleared imported row restored")
	}
}

func TestRestoreQueueSkipsForgotten(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	unpack.upsertHistory(HistoryRecord{
		ID: "kept", Kind: string(starr.Sonarr), Path: "/dl/kept",
		Status: IMPORTED, Updated: time.Now(), Forgotten: true,
		NewFiles: []string{"/dl/kept/ep.mkv"}, DeleteDelay: "1s",
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map["kept"] != nil {
		t.Fatal("forgotten item restored into map")
	}

	if !unpack.isForgotten("kept") {
		t.Fatal("forgotten tombstone not rehydrated")
	}
}

func TestForgetPersistsAndSkipsRestore(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	now := time.Now()
	item := &Extract{
		App: starr.Sonarr, Path: "/dl/show", Status: IMPORTED, Updated: now,
		Resp:        &xtractr.Response{NewFiles: []string{"/dl/show/ep.mkv"}},
		DeleteDelay: time.Minute,
	}
	unpack.Map["show"] = item
	unpack.maybeRecordHistory("show", item)

	if err := unpack.forgetQueueID("show"); err != nil {
		t.Fatal(err)
	}

	if unpack.Map["show"] != nil {
		t.Fatal("still in map")
	}

	if len(unpack.records) != 1 || !unpack.records[0].Forgotten {
		t.Fatalf("history forgotten flag %+v", unpack.records)
	}

	unpack.Map = make(map[string]*Extract)
	unpack.forgotten = make(map[string]struct{})
	unpack.restoreQueueFromHistory()

	if unpack.Map["show"] != nil {
		t.Fatal("forgotten imported item resurrected")
	}

	if !unpack.isForgotten("show") {
		t.Fatal("tombstone missing after restore")
	}
}

func TestRestoreQueueFolderExtracted(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	watch := t.TempDir()
	cfg := &FolderConfig{Path: watch}
	unpack.folders.Config = []*FolderConfig{cfg}
	name := filepath.Join(watch, "movie")
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: EXTRACTED, Updated: time.Now().Add(-time.Minute), Retries: 1,
		NewFiles:  []string{filepath.Join(name, "ep.mkv")},
		OrigFiles: []string{filepath.Join(name, "movie.rar")},
		PreFiles:  []string{filepath.Join(name, "keep.txt")},
	})
	unpack.restoreQueueFromHistory()

	item := unpack.Map[name]
	if item == nil || item.Status != EXTRACTED || item.App != FolderString || item.Retries != 1 {
		t.Fatalf("map %+v", item)
	}

	folder := unpack.folders.Folders[name]
	if folder == nil || folder.Status != EXTRACTED || folder.Retries != 1 || folder.Config != cfg {
		t.Fatalf("tracker %+v", folder)
	}

	if len(folder.Files) != 1 || len(folder.Archives.List()) != 1 {
		t.Fatalf("files %+v archives %+v", folder.Files, folder.Archives)
	}

	if _, ok := folder.PreFiles[filepath.Join(name, "keep.txt")]; !ok {
		t.Fatalf("preFiles %+v", folder.PreFiles)
	}
}

func TestRestoreQueueFolderInterruptedRetries(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	watch := t.TempDir()
	cfg := &FolderConfig{Path: watch}
	unpack.folders.Config = []*FolderConfig{cfg}
	name := filepath.Join(watch, "bad.zip")
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: EXTRACTING, Updated: time.Now(), Retries: 1,
		PreFiles: []string{filepath.Join(name, "keep.txt")},
	})
	unpack.restoreQueueFromHistory()

	item := unpack.Map[name]
	if item == nil || item.Status != EXTRACTFAILED || item.Retries != 1 {
		t.Fatalf("map %+v", item)
	}

	folder := unpack.folders.Folders[name]
	if folder == nil || folder.Status != EXTRACTFAILED || folder.Retries != 1 {
		t.Fatalf("tracker %+v", folder)
	}

	if _, ok := folder.PreFiles[filepath.Join(name, "keep.txt")]; !ok {
		t.Fatalf("preFiles %+v", folder.PreFiles)
	}

	unpack.checkFolderStats(time.Now())

	if folder = unpack.folders.Folders[name]; folder == nil || folder.Status != WAITING || folder.Retries != 2 {
		t.Fatalf("retry %+v", folder)
	}

	if item = unpack.Map[name]; item == nil || item.Status != WAITING || item.Retries != 2 {
		t.Fatalf("retry map %+v", item)
	}
}

func TestRestoreQueueFolderWatchPathGone(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	unpack.folders.Config = []*FolderConfig{{Path: t.TempDir()}}
	unpack.upsertHistory(HistoryRecord{
		ID: "/other/movie", Kind: FolderString, App: FolderString, Path: "/other/movie",
		Status: EXTRACTED, Updated: time.Now(),
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map["/other/movie"] != nil {
		t.Fatal("restored folder for a path that is not watched")
	}

	if unpack.folders.Folders["/other/movie"] != nil {
		t.Fatal("tracker has unmatched folder")
	}
}

func TestSeedFolderTrackerDropsUnwatched(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	name := filepath.Join(t.TempDir(), "movie")
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: EXTRACTED, Updated: time.Now(),
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map[name] == nil {
		t.Fatal("folder should stay in map until the watcher exists")
	}

	unpack.seedFolderTracker()

	if unpack.Map[name] != nil || unpack.folders.Folders[name] != nil {
		t.Fatal("unwatched restored folder should drop after PollFolders")
	}
}

func TestRestoreQueueFolderImportedSkipped(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	watch := t.TempDir()
	unpack.folders.Config = []*FolderConfig{{Path: watch}}
	name := filepath.Join(watch, "movie")
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: IMPORTED, Updated: time.Now(),
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map[name] != nil {
		t.Fatal("folder imported row restored")
	}
}

func TestRestoreQueueForgottenFolderNotRestored(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	watch := t.TempDir()
	name := filepath.Join(watch, "movie")
	unpack.folders.Config = []*FolderConfig{{Path: watch}}
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: EXTRACTED, Updated: time.Now(), Forgotten: true,
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map[name] != nil {
		t.Fatal("forgotten folder restored")
	}

	if unpack.isForgotten(name) {
		t.Fatal("folder forget should not rehydrate a Starr tombstone")
	}
}

func TestRestoreQueueFolderQueuedBecomesWaiting(t *testing.T) {
	t.Parallel()

	unpack := restoreTestUnpackerr(t)
	watch := t.TempDir()
	unpack.folders.Config = []*FolderConfig{{Path: watch}}
	name := filepath.Join(watch, "movie.rar")
	unpack.upsertHistory(HistoryRecord{
		ID: name, Kind: FolderString, App: FolderString, Path: name,
		Status: QUEUED, Updated: time.Now(),
	})
	unpack.restoreQueueFromHistory()

	item := unpack.Map[name]
	folder := unpack.folders.Folders[name]

	if item == nil || item.Status != WAITING || folder == nil || folder.Status != WAITING {
		t.Fatalf("queued folder %+v tracker %+v", item, folder)
	}
}

func TestMaybeRecordHistoryWritesOrigFiles(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	unpack.maybeRecordHistory("/watch/a", &Extract{
		App:     FolderString,
		Path:    "/watch/a",
		Status:  EXTRACTED,
		Updated: time.Now(),
		Resp: &xtractr.Response{
			NewFiles: []string{"/watch/a/ep.mkv"},
			Archives: xtractr.ArchiveList{"/watch/a": []string{"/watch/a/a.rar"}},
		},
	})

	if len(unpack.records) != 1 || len(unpack.records[0].OrigFiles) != 1 ||
		unpack.records[0].OrigFiles[0] != "/watch/a/a.rar" {
		t.Fatalf("%+v", unpack.records)
	}
}

func TestFolderConfigForPathPrefersLongerWatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	parent := &FolderConfig{Path: root}
	child := &FolderConfig{Path: filepath.Join(root, "tv")}
	got := folderConfigForPath([]*FolderConfig{parent, child}, filepath.Join(root, "tv", "show"))

	if got != child {
		t.Fatalf("got %+v", got)
	}
}
