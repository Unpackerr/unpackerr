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
