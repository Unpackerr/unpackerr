package unpackerr

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueueRetryAndForget(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map["/dl/fail"] = &Extract{Path: "/dl/fail", Status: EXTRACTFAILED, NoRetry: true}
	unpack.Map["/dl/live"] = &Extract{Path: "/dl/live", Status: EXTRACTING}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	retryOK := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"/dl/fail"}`, withKey)
	if retryOK.Code != http.StatusOK {
		t.Fatalf("retry %d %s", retryOK.Code, retryOK.Body.String())
	}

	if item := unpack.Map["/dl/fail"]; item.Status != WAITING || item.NoRetry || item.Retries != 1 {
		t.Fatalf("retry state %+v", item)
	}

	conflict := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"/dl/live"}`, withKey)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("retry live %d", conflict.Code)
	}

	forgetOK := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/dl/live"}`, withKey)
	if forgetOK.Code != http.StatusOK {
		t.Fatalf("forget %d %s", forgetOK.Code, forgetOK.Body.String())
	}

	if _, exists := unpack.Map["/dl/live"]; exists {
		t.Fatal("forgotten item still in map")
	}

	missing := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/nope"}`, withKey)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("forget missing %d", missing.Code)
	}
}

func TestHistoryDeleteAndClear(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "b", Path: "b", Status: DELETED, Updated: time.Now()})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	del := doAuth(t, unpack, http.MethodPost, "/api/history/delete", `{"id":"a"}`, withKey)
	if del.Code != http.StatusOK {
		t.Fatalf("delete %d %s", del.Code, del.Body.String())
	}

	left := unpack.historySnapshot()
	if len(left) != 1 || left[0].ID != "b" {
		t.Fatalf("after delete %+v", left)
	}

	cleared := doAuth(t, unpack, http.MethodPost, "/api/history/clear", "", withKey)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear %d", cleared.Code)
	}

	if len(unpack.historySnapshot()) != 0 {
		t.Fatal("history not cleared")
	}
}

func TestQueueWritePermission(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	statKey := strings.Repeat("Q", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "home",
		Key:   statKey,
		Roles: []string{"stats"},
	})

	if rec := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"x"}`, func(req *http.Request) {
		req.Header.Set(headerAPIKey, statKey)
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("stats key retry %d", rec.Code)
	}
}
