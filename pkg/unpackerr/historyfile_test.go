package unpackerr

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	text := string(body)
	if !strings.Contains(text, `"id":"c"`) || strings.Contains(text, `"id":"a"`) {
		t.Fatalf("file %s", text)
	}

	unpack.records = nil
	unpack.loadHistory()

	if len(unpack.historySnapshot()) != 2 {
		t.Fatal("reload")
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

func TestHistoryFilePathFallsBackToHome(t *testing.T) {
	t.Parallel()

	got := New().historyFilePath()
	if !strings.Contains(got, historyFileName) || !strings.Contains(got, ".unpackerr") {
		t.Fatalf("home fallback %q", got)
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

func TestLoadHistoryKeepsRecordsAfterBadLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), historyFileName)
	line := func(id string) string {
		return `{"id":"` + id + `","path":"` + id + `","status":"imported"}` + "\n"
	}

	body := line("a") + strings.Repeat("x", historyScanMax+16) + "\n" + line("c")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = path
	unpack.loadHistory()

	got := unpack.historySnapshot()
	if len(got) != 2 || got[0].ID != "c" || got[1].ID != "a" {
		t.Fatalf("%+v", got)
	}
}
