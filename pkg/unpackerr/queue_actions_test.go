package unpackerr

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/starr/radarr"
)

func TestQueueRetryAndForget(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map["/dl/fail"] = &Extract{Path: "/dl/fail", Status: EXTRACTFAILED, NoRetry: true}
	unpack.Map["/dl/live"] = &Extract{Path: "/dl/live", Status: EXTRACTING}
	unpack.Map["/dl/done"] = &Extract{Path: "/dl/done", Status: EXTRACTFAILED, NoRetry: true}

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

	forgetLive := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/dl/live"}`, withKey)
	if forgetLive.Code != http.StatusConflict {
		t.Fatalf("forget live %d %s", forgetLive.Code, forgetLive.Body.String())
	}

	if _, exists := unpack.Map["/dl/live"]; !exists {
		t.Fatal("in-progress item should remain until it is terminal")
	}

	forgetOK := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/dl/done"}`, withKey)
	if forgetOK.Code != http.StatusOK {
		t.Fatalf("forget %d %s", forgetOK.Code, forgetOK.Body.String())
	}

	if _, exists := unpack.Map["/dl/done"]; exists {
		t.Fatal("forgotten item still in map")
	}

	missing := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/nope"}`, withKey)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("forget missing %d", missing.Code)
	}
}

func TestQueueRetryFolder(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map["/watch/fail"] = &Extract{
		App:     FolderString,
		Path:    "/watch/fail",
		Status:  EXTRACTFAILED,
		NoRetry: true,
		Retries: 3,
	}
	unpack.folders = &Folders{Folders: map[string]*Folder{
		"/watch/fail": {status: EXTRACTFAILED, noRetry: true, retries: 99, updated: time.Now()},
	}}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	retryOK := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"/watch/fail"}`, withKey)
	if retryOK.Code != http.StatusOK {
		t.Fatalf("retry %d %s", retryOK.Code, retryOK.Body.String())
	}

	item := unpack.Map["/watch/fail"]
	if item.Status != WAITING || item.NoRetry || item.Retries != 3 || unpack.Retries != 0 {
		t.Fatalf("folder extract retry %+v totals %d", item, unpack.Retries)
	}

	folder := unpack.folders.Folders["/watch/fail"]
	if folder == nil || folder.status != WAITING || folder.noRetry || folder.retries != 0 {
		t.Fatalf("folder retry %+v", folder)
	}
}

func TestQueueForgetFolder(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map["/watch/gone"] = &Extract{App: FolderString, Path: "/watch/gone", Status: EXTRACTFAILED}
	unpack.folders = &Folders{Folders: map[string]*Folder{
		"/watch/gone": {status: EXTRACTFAILED},
	}}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	forgetOK := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/watch/gone"}`, withKey)
	if forgetOK.Code != http.StatusOK {
		t.Fatalf("forget %d %s", forgetOK.Code, forgetOK.Body.String())
	}

	if _, exists := unpack.Map["/watch/gone"]; exists {
		t.Fatal("forgotten folder still in map")
	}

	if _, exists := unpack.folders.Folders["/watch/gone"]; exists {
		t.Fatal("forgotten folder still tracked")
	}

	if unpack.isForgotten("/watch/gone") {
		t.Fatal("folder forget should not tombstone")
	}
}

func TestQueueIDJSON(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map[" /dl/spaced "] = &Extract{Path: " /dl/spaced ", Status: EXTRACTFAILED}
	unpack.Map["/dl/fail"] = &Extract{Path: "/dl/fail", Status: EXTRACTFAILED}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	extra := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"/dl/fail"}{"id":"y"}`, withKey)
	if extra.Code != http.StatusBadRequest {
		t.Fatalf("extra json %d %s", extra.Code, extra.Body.String())
	}

	if unpack.Map["/dl/fail"].Status != EXTRACTFAILED {
		t.Fatal("extra json should not retry")
	}

	unknown := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"/dl/fail","x":1}`, withKey)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field %d", unknown.Code)
	}

	blank := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":"   "}`, withKey)
	if blank.Code != http.StatusBadRequest {
		t.Fatalf("whitespace id %d", blank.Code)
	}

	spaced := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", `{"id":" /dl/spaced "}`, withKey)
	if spaced.Code != http.StatusOK {
		t.Fatalf("spaced id %d %s", spaced.Code, spaced.Body.String())
	}

	if unpack.Map[" /dl/spaced "].Status != WAITING {
		t.Fatal("path with spaces should retry using the original id")
	}
}

func TestForgottenStarrTitle(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Radarr = []*RadarrConfig{{
		Protocols: defaultProtocol,
		Queue: &radarr.Queue{Records: []*radarr.QueueRecord{{
			Title:      "Movie",
			Status:     "completed",
			Protocol:   starr.Protocol("torrent"),
			OutputPath: "/dl/Movie",
		}}},
	}}
	unpack.Map["Movie"] = &Extract{App: starr.Radarr, Path: "/dl/Movie", Status: EXTRACTFAILED, NoRetry: true}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	forgetOK := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"Movie"}`, withKey)
	if forgetOK.Code != http.StatusOK {
		t.Fatalf("forget %d %s", forgetOK.Code, forgetOK.Body.String())
	}

	unpack.checkRadarrQueue(time.Now())

	if _, exists := unpack.Map["Movie"]; exists {
		t.Fatal("forgotten title recreated from Starr queue")
	}

	unpack.Radarr[0].Queue.Records = nil
	unpack.sweepForgotten()

	unpack.Radarr[0].Queue.Records = []*radarr.QueueRecord{{
		Title:      "Movie",
		Status:     "completed",
		Protocol:   starr.Protocol("torrent"),
		OutputPath: "/dl/Movie",
	}}
	unpack.checkRadarrQueue(time.Now())

	if _, exists := unpack.Map["Movie"]; !exists {
		t.Fatal("title should track again after leaving the Starr queue")
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

	if _, err := os.Stat(unpack.histPath + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("history bak: %v", err)
	}

	cleared := doAuth(t, unpack, http.MethodPost, "/api/history/clear", "", withKey)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear %d", cleared.Code)
	}

	if len(unpack.historySnapshot()) != 0 {
		t.Fatal("history not cleared")
	}
}

func TestHistoryWriteFailureRestores(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix directory permissions")
	}

	unpack := testAuthUnpackerr(t)
	unpack.KeepHistory = 10

	dir := t.TempDir()

	sub := filepath.Join(dir, "hist")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	unpack.histPath = filepath.Join(sub, historyFileName)
	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: IMPORTED, Updated: time.Now()})
	unpack.upsertHistory(HistoryRecord{ID: "b", Path: "b", Status: DELETED, Updated: time.Now()})

	if err := os.Chmod(sub, 0o555); err != nil { //nolint:gosec // need a read-only dir
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) }) //nolint:gosec // restore after the read-only test

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	del := doAuth(t, unpack, http.MethodPost, "/api/history/delete", `{"id":"a"}`, withKey)
	if del.Code != http.StatusInternalServerError {
		t.Fatalf("delete fail %d %s", del.Code, del.Body.String())
	}

	left := unpack.historySnapshot()
	if len(left) != 2 {
		t.Fatalf("delete should restore %+v", left)
	}

	cleared := doAuth(t, unpack, http.MethodPost, "/api/history/clear", "", withKey)
	if cleared.Code != http.StatusInternalServerError {
		t.Fatalf("clear fail %d %s", cleared.Code, cleared.Body.String())
	}

	if len(unpack.historySnapshot()) != 2 {
		t.Fatal("clear should restore")
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
