package unpackerr

import (
	"bytes"
	"net/http"
	"path/filepath"
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

func TestQueueForgetImportedAndDeleted(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer

	unpack := testAuthUnpackerr(t)
	unpack.Info.SetOutput(&logs)
	unpack.Map["/dl/imp"] = &Extract{Path: "/dl/imp", Status: IMPORTED, App: starr.Sonarr}
	unpack.Map["/dl/gone"] = &Extract{Path: "/dl/gone", Status: DELETED, App: starr.Sonarr}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	imp := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/dl/imp"}`, withKey)
	if imp.Code != http.StatusOK {
		t.Fatalf("forget imported %d %s", imp.Code, imp.Body.String())
	}

	if unpack.Finished != 0 {
		t.Fatalf("imported forget must not count finished: %d", unpack.Finished)
	}

	if _, exists := unpack.Map["/dl/imp"]; exists {
		t.Fatal("imported item still in map")
	}

	logged := logs.String()
	if !strings.Contains(logged, "User forgot imported item") ||
		!strings.Contains(logged, "skipping file cleanup") ||
		!strings.Contains(logged, "/dl/imp") {
		t.Fatalf("imported forget log: %s", logged)
	}

	gone := doAuth(t, unpack, http.MethodPost, "/api/queue/forget", `{"id":"/dl/gone"}`, withKey)
	if gone.Code != http.StatusOK {
		t.Fatalf("forget deleted %d %s", gone.Code, gone.Body.String())
	}

	if unpack.Finished != 1 {
		t.Fatalf("deleted forget should count finished, got %d", unpack.Finished)
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
		"/watch/fail": {Status: EXTRACTFAILED, NoRetry: true, Retries: 99, Updated: time.Now()},
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
	if folder == nil || folder.Status != WAITING || folder.NoRetry || folder.Retries != 0 {
		t.Fatalf("folder retry %+v", folder)
	}
}

func TestQueueForgetFolder(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Map["/watch/gone"] = &Extract{App: FolderString, Path: "/watch/gone", Status: EXTRACTFAILED}
	unpack.folders = &Folders{Folders: map[string]*Folder{
		"/watch/gone": {Status: EXTRACTFAILED},
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

	oversized := `{"id":"` + strings.Repeat("x", maxActionBody) + `"}`

	tooBig := doAuth(t, unpack, http.MethodPost, "/api/queue/retry", oversized, withKey)
	if tooBig.Code != http.StatusBadRequest {
		t.Fatalf("oversized json %d %s", tooBig.Code, tooBig.Body.String())
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

	checkStarrQueue(unpack, unpack.Radarr, starr.Radarr, time.Now())

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
	checkStarrQueue(unpack, unpack.Radarr, starr.Radarr, time.Now())

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
