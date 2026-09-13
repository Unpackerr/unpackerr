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
	unpack.Sonarr = []*SonarrConfig{{}}
	unpack.Sonarr[0].Name = "Sportarr"
	unpack.Sonarr[0].URL = "http://sonarr:8989"

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

func TestRestoreQueueSkipsOldDeletedAndFolder(t *testing.T) {
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
	unpack.upsertHistory(HistoryRecord{
		ID: "/watch/folder", Kind: FolderString, App: FolderString, Path: "/watch/folder",
		Status: EXTRACTED, Updated: now,
	})
	unpack.restoreQueueFromHistory()

	if unpack.Map["old"] != nil || unpack.Map["gone"] != nil || unpack.Map["/watch/folder"] != nil {
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
	unpack.Sonarr = []*SonarrConfig{{}}
	unpack.Sonarr[0].URL = "http://sonarr:8989"

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
