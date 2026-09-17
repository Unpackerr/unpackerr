package unpackerr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golift.io/xtractr"
)

func TestGetFilePathsAndLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	current := filepath.Join(dir, "unpackerr.log")
	rotated := filepath.Join(dir, "unpackerr-old.log")

	if err := os.WriteFile(current, []byte("one\ntwo\nthree\n"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(rotated, []byte("old\n"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	infos := GetFilePaths(current)
	if len(infos.List) != 2 {
		t.Fatalf("files %d", len(infos.List))
	}

	var used *LogFileInfo

	for _, item := range infos.List {
		if item.Used {
			used = item
		}
	}

	if used == nil || used.Name != "unpackerr.log" {
		t.Fatalf("used %+v", used)
	}

	lines, err := getLinesFromFile(current, 2, 0)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(lines)); got != "two\nthree" {
		t.Fatalf("tail %q", got)
	}

	skipped, err := getLinesFromFile(current, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(skipped)); got != "two" {
		t.Fatalf("skip %q", got)
	}
}

func TestLogFileIDRejectsUnknown(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	rec := doAuth(t, unpack, http.MethodGet, "/api/logs/not-a-real-id", "", withKey)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id %d %s", rec.Code, rec.Body.String())
	}

	list := doAuth(t, unpack, http.MethodGet, "/api/logs", "", withKey)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}

	var infos LogFileInfos
	if err := json.Unmarshal(list.Body.Bytes(), &infos); err != nil {
		t.Fatal(err)
	}

	if len(infos.List) != 2 || infos.List[0].ID != errorLogID || infos.List[1].ID != liveLogID {
		t.Fatalf("synthetic logs %+v", infos.List)
	}
}

func TestRecentErrorsLog(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	for i := range errorRingMax + 2 {
		unpack.hub.notifyError(fmt.Sprintf("err-%d", i))
	}

	rec := doAuth(t, unpack, http.MethodGet, "/api/logs/"+errorLogID, "", withKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("errors %d %s", rec.Code, rec.Body.String())
	}

	var body logLinesBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(body.Text, "err-11") || strings.Contains(body.Text, "err-0") {
		t.Fatalf("ring %q", body.Text)
	}

	dl := doAuth(t, unpack, http.MethodGet, "/api/logs/"+errorLogID+"/download", "", withKey)
	if dl.Code != http.StatusBadRequest {
		t.Fatalf("download %d %s", dl.Code, dl.Body.String())
	}
}

func TestLogFileInfosKeepsLiveWithHTTPLog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	httpLog := filepath.Join(dir, "http.log")
	if err := os.WriteFile(httpLog, []byte("req\n"), defaultFileMode); err != nil {
		t.Fatal(err)
	}

	unpack := New()
	unpack.Webserver.LogFile = httpLog

	infos := unpack.logFileInfos()
	if len(infos.List) < 3 || infos.List[0].ID != errorLogID || infos.List[1].ID != liveLogID {
		t.Fatalf("live hidden %+v", infos.List)
	}

	if unpack.findLogFile(liveLogID) == nil {
		t.Fatal("live id missing")
	}
}

func TestLogFollowSnapshotUnknownID(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.appLogTee = newLogTee(unpack.hub, liveLogID)
	unpack.appLogTee.ring.add("app-line")

	if got := unpack.logFollowSnapshot("not-a-real-id", 50); got != nil {
		t.Fatalf("unknown id leaked tee %q", got)
	}
}

func TestQueueFromExtractProgressFields(t *testing.T) {
	t.Parallel()

	eta := time.Unix(1_700_000_100, 0)
	item := &Extract{
		Path:    "/dl/show",
		App:     "Sonarr",
		Status:  EXTRACTING,
		Updated: time.Now(),
		XProg: &ExtractProgress{
			Extract:   &Extract{Path: "/dl/show"},
			Archives:  3,
			Extracted: 1,
			SpeedBps:  42,
			ETA:       eta,
			Progress: &xtractr.Progress{
				Total:      100,
				Wrote:      25,
				Compressed: 50,
				Read:       10,
				Files:      2,
				Count:      8,
				XFile:      &xtractr.XFile{FilePath: "/dl/show/a.rar"},
			},
		},
	}
	item.XProg.Extract = item

	got := queueFromExtract("Show.Name", item)
	if got.ID != "Show.Name" || got.Percent != 25 || got.Wrote != 25 || got.Total != 100 ||
		got.Archives != 3 || got.Extracted != 1 || got.Archive != "a.rar" ||
		got.SpeedBps != 42 || !got.ETA.Equal(eta) {
		t.Fatalf("%+v", got)
	}

	item.Path = `C:\dl\show`
	item.XProg.XFile.FilePath = `C:\dl\show\a.rar`

	got = queueFromExtract("Show.Name", item)
	if got.Archive != "a.rar" {
		t.Fatalf("backslash archive %q", got.Archive)
	}
}

func TestLiveHubDropsUnknownTopic(t *testing.T) {
	t.Parallel()

	info := authInfo{Permissions: []string{PermReadSystemQueue}}
	allowed := allowedTopics(info)

	if strings.Contains(strings.Join(allowed, ","), topicLogs) {
		t.Fatal("logs should be denied")
	}

	if !strings.Contains(strings.Join(allowed, ","), topicQueue) {
		t.Fatal("queue should be allowed")
	}

	if topicPerm("nope") != "unknown" {
		t.Fatal("unknown topic")
	}
}

func TestWSRequiresAuth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodGet, "/ws", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("ws %d %s", rec.Code, rec.Body.String())
	}
}

func TestLogFileFolders(t *testing.T) {
	t.Parallel()

	same := t.TempDir()
	other := t.TempDir()
	unpack := New()
	unpack.LogFile = filepath.Join(same, "unpackerr.log")
	unpack.Webserver.LogFile = filepath.Join(same, "http.log")

	if got := unpack.logFileFolders(); got != same {
		t.Fatalf("same dir %q, want %q", got, same)
	}

	unpack.Webserver.LogFile = filepath.Join(other, "http.log")

	want := same + ", " + other
	if got := unpack.logFileFolders(); got != want {
		t.Fatalf("diff dirs %q, want %q", got, want)
	}

	unpack.LogFile = "unpackerr.log"
	unpack.Webserver.LogFile = ""

	if got := unpack.logFileFolders(); got != "." {
		t.Fatalf("relative %q", got)
	}

	unpack.LogFile = ""

	if got := unpack.logFileFolders(); got != "" {
		t.Fatalf("empty %q", got)
	}
}
