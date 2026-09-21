package unpackerr

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/xtractr"
)

func TestHistoryUpsertAndCap(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 2
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: EXTRACTFAILED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "b", Path: "b", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "c", Path: "c", Status: DELETED, Updated: time.Now()})

	got := unpack.historySnapshot()
	if len(got) != 2 {
		t.Fatalf("cap: %d", len(got))
	}

	if got[0].ID != "c" || got[1].ID != "b" {
		t.Fatalf("newest first: %+v", got)
	}

	body, err := os.ReadFile(unpack.histPath)
	if err != nil {
		t.Fatal(err)
	}

	// Append-only: the file still holds the over-cap row until it is compacted.
	if text := string(body); !strings.Contains(text, `"id":"c"`) || !strings.Contains(text, `"id":"a"`) {
		t.Fatalf("file %s", text)
	}

	unpack.records = nil
	unpack.loadHistory()

	if len(unpack.historySnapshot()) != 2 {
		t.Fatal("reload")
	}

	// Load folds the file back to the cap.
	if body, err = os.ReadFile(unpack.histPath); err != nil {
		t.Fatal(err)
	}

	if text := string(body); strings.Contains(text, `"id":"a"`) || strings.Count(text, "\n") != 2 {
		t.Fatalf("compacted file %s", text)
	}
}

func TestQueueAndHistoryAPI(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.KeepHistory = 10
	unpack.Map["/dl/show"] = &Extract{
		Path:    "/dl/show",
		App:     "Sonarr",
		Status:  EXTRACTING,
		Updated: time.Now(),
	}
	unpack.upsertHistory(HistoryRecord{ID: "/dl/old", Path: "/dl/old", Status: IMPORTED})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	queueRec := doAuth(t, unpack, http.MethodGet, "/api/queue", "", withKey)
	if queueRec.Code != http.StatusOK || !strings.Contains(queueRec.Body.String(), `/dl/show`) {
		t.Fatalf("queue %d %s", queueRec.Code, queueRec.Body.String())
	}

	histRec := doAuth(t, unpack, http.MethodGet, "/api/history", "", withKey)
	if histRec.Code != http.StatusOK || !strings.Contains(histRec.Body.String(), `/dl/old`) {
		t.Fatalf("history %d %s", histRec.Code, histRec.Body.String())
	}

	itemRec := doAuth(t, unpack, http.MethodGet, "/api/queue/item?id=/dl/show", "", withKey)
	if itemRec.Code != http.StatusOK || !strings.Contains(itemRec.Body.String(), `/dl/show`) {
		t.Fatalf("queue item %d %s", itemRec.Code, itemRec.Body.String())
	}

	missing := doAuth(t, unpack, http.MethodGet, "/api/queue/item?id=/nope", "", withKey)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing %d %s", missing.Code, missing.Body.String())
	}

	if blank := doAuth(t, unpack, http.MethodGet, "/api/queue/item", "", withKey); blank.Code != http.StatusBadRequest {
		t.Fatalf("blank %d %s", blank.Code, blank.Body.String())
	}
}

func TestHistoryFilePathSkipsUnopenedLog(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.LogFile = filepath.Join("missing", "unpackerr.log")
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	got := unpack.historyFilePath()
	want := filepath.Join(filepath.Dir(unpack.ConfigFile), historyFileName)

	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHistoryFilePathEmptyWithoutLogOrConfig(t *testing.T) {
	t.Parallel()

	if got := New().historyFilePath(); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestHistoryStaysInMemoryWithoutLogOrConfig(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.loadHistory()

	if unpack.histPath != "" {
		t.Fatalf("histPath %q", unpack.histPath)
	}

	unpack.maybeRecordHistory("/dl/done", &Extract{
		Path:    "/dl/done",
		Status:  IMPORTED,
		Updated: time.Now(),
	})

	got := unpack.historySnapshot()
	if len(got) != 1 || got[0].ID != "/dl/done" {
		t.Fatalf("%+v", got)
	}
}

func TestHistoryIDMatchesQueueMapKey(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	const title = "Show.S01E01"

	path := "/downloads/Show.S01E01.Group"
	unpack.maybeRecordHistory(title, &Extract{
		App:     "Sonarr",
		Path:    path,
		Status:  IMPORTED,
		Updated: time.Now(),
	})

	got := unpack.historySnapshot()
	if len(got) != 1 || got[0].ID != title || got[0].Path != path {
		t.Fatalf("%+v", got)
	}
}

func TestLoadHistoryRewritesFileCap(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "b", Path: "b", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "c", Path: "c", Status: IMPORTED, Updated: time.Now()})

	unpack.KeepHistory = 1
	unpack.records = nil
	unpack.loadHistory()

	if got := unpack.historySnapshot(); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("memory %+v", unpack.historySnapshot())
	}

	body, err := os.ReadFile(unpack.histPath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(string(body), `"id":`) != 1 || !strings.Contains(string(body), `"id":"c"`) {
		t.Fatalf("file %s", body)
	}
}

func TestCapHistoryKeepsExtracted(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 2
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	unpack.upsertHistory(HistoryRecord{ID: "show", Path: "/dl/show", Status: EXTRACTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "b", Path: "b", Status: IMPORTED, Updated: time.Now()})

	if len(unpack.records) != 3 {
		t.Fatalf("cap %+v", unpack.records)
	}

	got := unpack.historySnapshot()
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("history %+v", got)
	}

	ids := map[string]ExtractStatus{}
	for _, rec := range unpack.records {
		ids[rec.ID] = rec.Status
	}

	if ids["show"] != EXTRACTED || ids["a"] != IMPORTED || ids["b"] != IMPORTED {
		t.Fatalf("kept %+v", unpack.records)
	}
}

func TestCapHistoryIgnoresInFlightCount(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 2
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)

	now := time.Now()
	for _, id := range []string{"q1", "q2", "q3"} {
		unpack.upsertHistory(HistoryRecord{ID: id, Path: "/" + id, Status: EXTRACTED, Updated: now})
	}

	unpack.upsertHistory(HistoryRecord{ID: "old", Path: "/old", Status: IMPORTED, Updated: now})
	unpack.upsertHistory(HistoryRecord{ID: "new", Path: "/new", Status: IMPORTED, Updated: now})
	unpack.upsertHistory(HistoryRecord{ID: "newer", Path: "/newer", Status: IMPORTED, Updated: now})

	ids := map[string]ExtractStatus{}
	for _, rec := range unpack.records {
		ids[rec.ID] = rec.Status
	}

	if len(unpack.records) != 5 {
		t.Fatalf("records %+v", unpack.records)
	}

	for _, id := range []string{"q1", "q2", "q3"} {
		if ids[id] != EXTRACTED {
			t.Fatalf("checkpoint %s missing %+v", id, unpack.records)
		}
	}

	if _, ok := ids["old"]; ok {
		t.Fatalf("oldest durable still present %+v", unpack.records)
	}

	got := unpack.historySnapshot()
	if len(got) != 2 || got[0].ID != "newer" || got[1].ID != "new" {
		t.Fatalf("history %+v", got)
	}
}

type queueDueCase struct {
	name   string
	itemID string
	item   *Extract
	kind   string
	due    time.Time
}

func dueCase(name, itemID, kind string, item *Extract, due time.Time) queueDueCase {
	return queueDueCase{name: name, itemID: itemID, item: item, kind: kind, due: due}
}

func TestQueueFromExtractFillsDue(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	unpack := New()
	unpack.StartDelay.Duration = time.Minute
	unpack.RetryDelay.Duration = 5 * time.Minute
	unpack.folders.Folders["/w"] = &Folder{
		Config: &FolderConfig{DeleteAfter: &cnfg.Duration{Duration: 10 * time.Minute}},
	}

	unstamped := unpack.queueFromExtract("a", &Extract{App: "Sonarr", Status: WAITING, Updated: now})
	if unstamped.DueKind != "" || !unstamped.Due.IsZero() {
		t.Fatalf("snapshot must copy stamped due only: %+v", unstamped)
	}

	tests := []queueDueCase{
		dueCase("start", "a", dueStart, &Extract{App: "Sonarr", Status: WAITING, Updated: now}, now.Add(time.Minute)),
		dueCase("noted", "a", "", &Extract{App: "Sonarr", Status: WAITING, Updated: now, Note: "x"}, time.Time{}),
		dueCase("folder start", "/w", dueStart,
			&Extract{App: FolderString, Status: WAITING, Updated: now, Note: "x"}, now.Add(time.Minute)),
		dueCase("retry", "a", dueRetry, &Extract{Status: EXTRACTFAILED, Updated: now}, now.Add(5*time.Minute)),
		dueCase("noretry", "a", "", &Extract{Status: EXTRACTFAILED, NoRetry: true, Updated: now}, time.Time{}),
		dueCase("cleanup", "/w", dueCleanup,
			&Extract{App: FolderString, Status: EXTRACTED, Updated: now}, now.Add(10*time.Minute)),
		dueCase("starr extracted", "a", "", &Extract{App: "Sonarr", Status: EXTRACTED, Updated: now}, time.Time{}),
		dueCase("imported", "a", dueCleanup,
			&Extract{Status: IMPORTED, Updated: now, DeleteDelay: 2 * time.Minute}, now.Add(2*time.Minute)),
		dueCase("skip del", "a", "", &Extract{Status: IMPORTED, Updated: now, DeleteDelay: -time.Second}, time.Time{}),
		dueCase("history", "a", dueHistory,
			&Extract{Status: DELETED, Updated: now, DeleteDelay: time.Minute}, now.Add(time.Minute)),
		dueCase("nothing", "/w", dueHistory,
			&Extract{App: FolderString, Status: EXTRACTEDNOTHING, Updated: now}, now.Add(time.Minute)),
		dueCase("starr nothing", "a", "", &Extract{App: "Sonarr", Status: EXTRACTEDNOTHING, Updated: now}, time.Time{}),
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			unpack.stampQueueDue(test.itemID, test.item)

			got := unpack.queueFromExtract(test.itemID, test.item)
			if got.DueKind != test.kind || !got.Due.Equal(test.due) {
				t.Fatalf("got kind %q due %v want %q %v", got.DueKind, got.Due, test.kind, test.due)
			}
		})
	}
}

func TestQueueFromExtractOmitsFileLists(t *testing.T) {
	t.Parallel()

	item := &Extract{
		Path: "/dl/a",
		App:  "Sonarr",
		IDs:  map[string]any{"title": "Show"},
		Resp: &xtractr.Response{
			NewFiles: []string{"/dl/a/file.mkv"},
			Size:     100,
			Started:  time.Unix(1_700_000_000, 0),
			Elapsed:  time.Second,
			Queued:   2,
			Output:   "/dl/a_unpackerred",
			Archives: xtractr.ArchiveList{"": []string{"/dl/a/file.rar"}},
			Extras:   xtractr.ArchiveList{"": []string{"/dl/a/nested.rar"}},
		},
	}

	got := New().queueFromExtract("/dl/a", item)
	if len(got.NewFiles) != 0 || len(got.OrigFiles) != 0 {
		t.Fatalf("progress payload must omit file lists: %+v", got)
	}

	if got.IDs["title"] != "Show" || got.Bytes != 100 || got.Queue != 2 || got.Output != "/dl/a_unpackerred" {
		t.Fatalf("meta %+v", got)
	}

	fillQueueFiles(&got, item)

	if len(got.NewFiles) != 1 || len(got.OrigFiles) != 2 {
		t.Fatalf("detail files %+v", got)
	}
}

func TestHistoryFromExtractCopiesHookMeta(t *testing.T) {
	t.Parallel()

	rec := historyFromExtract("/dl/a", &Extract{
		Path:     "/dl/a",
		App:      "Sonarr",
		HookFail: 4,
		Event:    "fsnotify",
		IDs:      map[string]any{"title": "Show"},
		Resp:     &xtractr.Response{Queued: 3, Size: 9, NewFiles: []string{"a"}, Output: "/tmp/out"},
	})
	if rec.Event != "fsnotify" || rec.Queue != 3 || rec.Output != "/tmp/out" || rec.IDs["title"] != "Show" {
		t.Fatalf("%+v", rec)
	}

	if rec.HookFail != 4 {
		t.Fatalf("hookFail %d", rec.HookFail)
	}
}

func TestHistoryFromExtractKeepsRestoredElapsed(t *testing.T) {
	t.Parallel()

	item := New().extractFromHistoryRecord(HistoryRecord{
		ID: "a", Path: "/dl/a", Status: EXTRACTED, Elapsed: "3s", Bytes: 1, NewFiles: []string{"a"},
		HookFail: 7,
	}, "Sonarr", EXTRACTED, "", time.Now(), time.Now())
	item.Status = IMPORTED

	got := historyFromExtract("a", item)
	if got.Elapsed != "3s" {
		t.Fatalf("elapsed %q", got.Elapsed)
	}

	if item.HookFail != 7 || got.HookFail != 7 {
		t.Fatalf("hookFail item %d rec %d", item.HookFail, got.HookFail)
	}
}
