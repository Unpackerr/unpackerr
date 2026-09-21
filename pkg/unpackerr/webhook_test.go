package unpackerr

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

var errHookExtract = errors.New("boom")

func hookUnpackerr(hooks HooksConfig) *Unpackerr {
	return &Unpackerr{Config: &Config{Hooks: hooks}}
}

func TestHookPayloadUsesLabel(t *testing.T) {
	t.Parallel()

	unpack := hookUnpackerr(HooksConfig{})

	payload := unpack.hookPayload(&Extract{App: starr.Sonarr, Name: "Sportarr", Path: "/dl"})
	if payload.App != "Sportarr" {
		t.Fatalf("payload.App = %q, want Sportarr", payload.App)
	}

	plain := unpack.hookPayload(&Extract{App: starr.Sonarr, Path: "/dl"})
	if plain.App != starr.Sonarr {
		t.Fatalf("unnamed payload.App = %q, want %s", plain.App, starr.Sonarr)
	}
}

func TestHookPayloadKeepsDataForImportedAndDeleted(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	resp := &xtractr.Response{
		NewFiles: []string{"/out/a.mkv"},
		Output:   "/tmp/out",
		Size:     42,
		Queued:   3,
		Started:  started,
		Elapsed:  time.Second,
		Archives: xtractr.ArchiveList{"/dl": {"/dl/a.rar"}},
		Error:    errHookExtract,
	}

	unpack := hookUnpackerr(HooksConfig{})

	for _, status := range []ExtractStatus{IMPORTED, DELETED} {
		payload := unpack.hookPayload(&Extract{
			App:    starr.Sonarr,
			Path:   "/dl",
			Status: status,
			Resp:   resp,
		})
		if payload.Data == nil {
			t.Fatalf("%s: data is nil", status)
		}

		if payload.Data.Bytes != 42 || payload.Data.Output != "/tmp/out" || payload.Data.Queue != 3 {
			t.Fatalf("%s: %+v", status, payload.Data)
		}

		if payload.Data.Error != errHookExtract.Error() {
			t.Fatalf("%s: error %q", status, payload.Data.Error)
		}

		if len(payload.Data.File) != 1 || payload.Data.File[0] != "/out/a.mkv" {
			t.Fatalf("%s: files %+v", status, payload.Data.File)
		}

		if len(payload.Data.Archives) != 1 || payload.Data.Archives[0] != "/dl/a.rar" {
			t.Fatalf("%s: archives %+v", status, payload.Data.Archives)
		}
	}
}

func TestHookPayloadOmitsDataWithoutResp(t *testing.T) {
	t.Parallel()

	payload := hookUnpackerr(HooksConfig{}).hookPayload(&Extract{App: starr.Sonarr, Path: "/dl", Status: IMPORTED})
	if payload.Data != nil {
		t.Fatalf("data = %+v", payload.Data)
	}
}

func TestHookPayloadRetriesAndEventTitle(t *testing.T) {
	t.Parallel()

	unpack := hookUnpackerr(HooksConfig{
		CustomIDs: map[string]string{
			"url":        "https://unpackerr.example",
			"title":      "ignored",
			"host":       "unpackerr",
			"downloadId": "nope",
		},
		Titles: HookTitles{Extracting: "Archive Found"},
	})

	payload := unpack.hookPayload(&Extract{
		App:     starr.Sonarr,
		Path:    "/dl",
		Status:  EXTRACTING,
		Retries: 3,
		IDs:     map[string]any{"title": "Show", "downloadId": "abc"},
	})

	if payload.Retries != 3 {
		t.Fatalf("retries %d", payload.Retries)
	}

	if payload.EventTitle != "Archive Found" {
		t.Fatalf("title %q", payload.EventTitle)
	}

	if payload.IDs["title"] != "Show" || payload.IDs["downloadId"] != "abc" {
		t.Fatalf("native ids clobbered: %+v", payload.IDs)
	}

	if _, ok := payload.IDs["url"]; ok {
		t.Fatalf("custom ids must not merge into ids: %+v", payload.IDs)
	}

	if payload.CustomIDs["url"] != "https://unpackerr.example" ||
		payload.CustomIDs["host"] != "unpackerr" ||
		payload.CustomIDs["title"] != "ignored" ||
		payload.CustomIDs["downloadId"] != "nope" {
		t.Fatalf("custom ids %+v", payload.CustomIDs)
	}

	plain := hookUnpackerr(HooksConfig{}).hookPayload(&Extract{Status: EXTRACTED})
	if plain.EventTitle != EXTRACTED.Desc() {
		t.Fatalf("default title %q", plain.EventTitle)
	}
}

func TestRecordHookFailIncrementsLiveAndHistory(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.Map["/dl/show"] = &Extract{
		Path:    "/dl/show",
		App:     starr.Sonarr,
		Status:  EXTRACTING,
		Updated: time.Now(),
	}

	unpack.recordHookFail("/dl/show")

	if unpack.Map["/dl/show"].HookFail != 1 {
		t.Fatalf("live %d", unpack.Map["/dl/show"].HookFail)
	}

	queue := unpack.queueFromExtract("/dl/show", unpack.Map["/dl/show"])
	if queue.HookFail != 1 {
		t.Fatalf("queue %d", queue.HookFail)
	}

	unpack.recordHookFail("/dl/show")

	if unpack.Map["/dl/show"].HookFail != 2 {
		t.Fatalf("live 2: %d", unpack.Map["/dl/show"].HookFail)
	}

	found := false

	for _, rec := range unpack.records {
		if rec.Path == "/dl/show" && rec.HookFail == 2 {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("history checkpoint %+v", unpack.records)
	}

	unpack.upsertHistory(HistoryRecord{ID: "/dl/old", Path: "/dl/old", Status: IMPORTED, HookFail: 1})
	delete(unpack.Map, "/dl/show")
	unpack.recordHookFail("/dl/old")

	snap := unpack.historySnapshot()
	if len(snap) == 0 || snap[0].Path != "/dl/old" || snap[0].HookFail != 2 {
		t.Fatalf("finished history %+v", snap)
	}
}

func TestRecordHookFailIgnoresEmptyPath(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Map["/dl"] = &Extract{Path: "/dl"}
	unpack.recordHookFail("")

	if unpack.Map["/dl"].HookFail != 0 {
		t.Fatal("empty path must not bump")
	}
}
