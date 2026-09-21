package unpackerr

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/hooks"
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

func TestRecordHookFailUsesMapIDNotPath(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Map["Show A"] = &Extract{Path: "/shared", App: starr.Sonarr, Status: EXTRACTING}
	unpack.Map["Show B"] = &Extract{Path: "/shared", App: starr.Sonarr, Status: EXTRACTING}

	unpack.recordHookFail("Show B")

	if unpack.Map["Show A"].HookFail != 0 {
		t.Fatal("same-path neighbor must not increment")
	}

	if unpack.Map["Show B"].HookFail != 1 {
		t.Fatalf("live %d", unpack.Map["Show B"].HookFail)
	}
}

func TestRecordHookFailHistoryMatchesID(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.upsertHistory(HistoryRecord{ID: "old", Path: "/shared", Status: IMPORTED, HookFail: 1})
	unpack.upsertHistory(HistoryRecord{ID: "new", Path: "/shared", Status: IMPORTED})

	unpack.recordHookFail("new")

	var oldFail, newFail uint

	for _, rec := range unpack.records {
		switch rec.ID {
		case "old":
			oldFail = rec.HookFail
		case "new":
			newFail = rec.HookFail
		}
	}

	if oldFail != 1 || newFail != 1 {
		t.Fatalf("old %d new %d", oldFail, newFail)
	}
}

func TestRecordHookFailDoesNotResurrectDeleted(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.upsertHistory(HistoryRecord{ID: "gone", Path: "/dl/gone", Status: IMPORTED, HookFail: 1})

	if err := unpack.deleteHistoryID("gone"); err != nil {
		t.Fatal(err)
	}

	unpack.recordHookFail("gone")

	if len(unpack.records) != 0 {
		t.Fatalf("resurrected %+v", unpack.records)
	}
}

func TestPendingHooksFlushAfterUnlock(t *testing.T) {
	t.Parallel()

	unpack := New()
	now := time.Now()

	unpack.lockHistory()
	unpack.updateQueueStatus(&newStatus{Name: "/dl/folder", Status: QUEUED}, now, true)
	unpack.updateQueueStatus(&newStatus{Name: "/dl/folder", Status: EXTRACTING}, now, true)

	if len(unpack.pendingHooks) != 2 {
		t.Fatalf("pending %d", len(unpack.pendingHooks))
	}

	if unpack.pendingHooks[0].item.Status != QUEUED || unpack.pendingHooks[1].item.Status != EXTRACTING {
		t.Fatalf("snapshots %s %s", unpack.pendingHooks[0].item.Status, unpack.pendingHooks[1].item.Status)
	}

	if unpack.Map["/dl/folder"].Status != EXTRACTING {
		t.Fatal("live extract moved on")
	}

	unpack.unlockHistory()

	if len(unpack.pendingHooks) != 0 {
		t.Fatalf("flushed %d", len(unpack.pendingHooks))
	}
}

func TestReportHookFailWaitsForDrain(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Map["Show A"] = &Extract{Path: "/shared", App: starr.Sonarr, Status: EXTRACTING}

	unpack.reportHookFail("Show A")

	if unpack.Map["Show A"].HookFail != 0 {
		t.Fatal("worker must not increment")
	}

	unpack.drainHookFails()

	if unpack.Map["Show A"].HookFail != 1 {
		t.Fatalf("drained %d", unpack.Map["Show A"].HookFail)
	}
}

func TestRecordHookFailBumpsHistoryWhenWaiting(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 10
	unpack.upsertHistory(HistoryRecord{ID: "Show", Path: "/shared", Status: EXTRACTFAILED, HookFail: 1})
	unpack.Map["Show"] = &Extract{Path: "/shared", App: starr.Sonarr, Status: WAITING, HookFail: 1}

	unpack.recordHookFail("Show")

	if unpack.Map["Show"].HookFail != 2 {
		t.Fatalf("live %d", unpack.Map["Show"].HookFail)
	}

	if len(unpack.records) != 1 || unpack.records[0].Status != EXTRACTFAILED || unpack.records[0].HookFail != 2 {
		t.Fatalf("checkpoint %+v", unpack.records)
	}
}

func TestBumpHistoryHookFailSkipsWhenHistoryOff(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.KeepHistory = 0
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.upsertHistory(HistoryRecord{ID: "old", Path: "/dl/old", Status: IMPORTED, HookFail: 1})

	unpack.recordHookFail("old")

	if unpack.records[0].HookFail != 1 {
		t.Fatalf("history off wrote %d", unpack.records[0].HookFail)
	}
}

func TestQueueHookIgnoresNil(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.queueHook("Show A", "", nil, nil)

	if unpack.inFlight.Load() != 0 {
		t.Fatalf("in-flight %d", unpack.inFlight.Load())
	}

	if unpack.hookWorker.Len() != 0 {
		t.Fatalf("queued %d", unpack.hookWorker.Len())
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

func TestSaveHookMessageLiveAndHistory(t *testing.T) {
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

	unpack.saveHookMessage("/dl/show", "discord", "msg-1", nil)

	if unpack.Map["/dl/show"].HookMessages["discord"] != "msg-1" {
		t.Fatalf("live %+v", unpack.Map["/dl/show"].HookMessages)
	}

	if unpack.hookMessage("/dl/show", "discord") != "msg-1" {
		t.Fatal("lookup live")
	}

	unpack.saveHookMessage("/dl/show", "discord", "msg-2", nil)

	found := false

	for _, rec := range unpack.records {
		if rec.Path == "/dl/show" && rec.HookMessages["discord"] == "msg-2" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("history checkpoint %+v", unpack.records)
	}

	unpack.upsertHistory(HistoryRecord{
		ID: "/dl/old", Path: "/dl/old", Status: IMPORTED,
		HookMessages: map[string]string{"discord": "stale"},
	})
	delete(unpack.Map, "/dl/show")
	unpack.saveHookMessage("/dl/old", "discord", "fresh", nil)

	if unpack.hookMessage("/dl/old", "discord") != "fresh" {
		t.Fatalf("history lookup %q", unpack.hookMessage("/dl/old", "discord"))
	}
}

func TestStoreHookMessageWaitsForDrain(t *testing.T) {
	t.Parallel()

	unpack := New()
	live := &Extract{Path: "/dl/show", App: starr.Sonarr, Status: EXTRACTING}
	unpack.Map["/dl/show"] = live
	unpack.seedHookMessages("/dl/show", map[string]string{"discord-1": "old"}, live)

	if unpack.lookupHookMessage("/dl/show", "discord-1", live) != "old" {
		t.Fatal("seed")
	}

	unpack.storeHookMessage("/dl/show", "discord-1", "msg-1", live)

	if live.HookMessages != nil {
		t.Fatal("worker must not persist")
	}

	if unpack.lookupHookMessage("/dl/show", "discord-1", live) != "msg-1" {
		t.Fatal("fifo lookup")
	}

	if unpack.idle() {
		t.Fatal("queued message ids must block a restart")
	}

	unpack.drainHookMessages()

	if live.HookMessages["discord-1"] != "msg-1" {
		t.Fatalf("drained %+v", live.HookMessages)
	}

	if unpack.lookupHookMessage("/dl/show", "discord-1", live) != "msg-1" {
		t.Fatal("cache after drain")
	}
}

func TestSaveHookMessageSkipsReusedExtract(t *testing.T) {
	t.Parallel()

	unpack := New()
	old := &Extract{Path: "/dl/show"}
	newer := &Extract{Path: "/dl/show"}
	unpack.Map["/dl/show"] = newer

	unpack.saveHookMessage("/dl/show", "discord-1", "msg-1", old)

	if newer.HookMessages["discord-1"] == "msg-1" {
		t.Fatal("must not attach to a reused extract")
	}
}

func TestQueueHookUsesInstanceSlug(t *testing.T) {
	t.Parallel()

	unpack := New()
	live := &Extract{Path: "/dl/show", Status: QUEUED}
	unpack.Map["Show"] = live
	hookURL := "https://discord.com/api/webhooks/1/x"
	item := &hooks.Item{Config: &hooks.Config{Name: hookURL, URL: hookURL}}

	unpack.queueHook("Show", "discord-1", live, item)
	item.SaveID("msg-1")

	if live.HookMessages != nil {
		t.Fatal("worker must not persist")
	}

	if unpack.lookupHookMessage("Show", "discord-1", live) != "msg-1" {
		t.Fatal("fifo lookup")
	}

	unpack.drainHookMessages()

	if live.HookMessages["discord-1"] != "msg-1" {
		t.Fatalf("slug key %+v", live.HookMessages)
	}

	if _, ok := live.HookMessages[hookURL]; ok {
		t.Fatal("url must not be the key")
	}
}

func TestHookMsgsDoesNotReuseStaleID(t *testing.T) {
	t.Parallel()

	unpack := New()
	old := &Extract{Path: "/dl/show"}
	unpack.Map["Show"] = old
	unpack.storeHookMessage("Show", "discord-1", "old-msg", old)
	unpack.drainHookMessages()
	unpack.deleteExtract("Show")

	if unpack.lookupHookMessage("Show", "discord-1", old) != "" {
		t.Fatal("finished extract must evict the cache")
	}

	newer := &Extract{Path: "/dl/show"}
	unpack.Map["Show"] = newer
	unpack.seedHookMessages("Show", nil, newer)

	if unpack.lookupHookMessage("Show", "discord-1", newer) != "" {
		t.Fatal("new extract must not edit the previous message")
	}

	unpack.storeHookMessage("Show", "discord-1", "late", old)

	if unpack.lookupHookMessage("Show", "discord-1", newer) != "" {
		t.Fatal("late save must not overwrite the new extract")
	}

	if newer.HookMessages["discord-1"] == "late" {
		t.Fatal("late save must not persist onto the new extract")
	}
}

func TestSeedReplacesMismatchedHookMsgOwner(t *testing.T) {
	t.Parallel()

	unpack := New()
	old := &Extract{Path: "/dl/show"}
	newer := &Extract{Path: "/dl/show"}

	unpack.storeHookMessage("Show", "discord-1", "old-msg", old)
	unpack.seedHookMessages("Show", nil, newer)

	if unpack.lookupHookMessage("Show", "discord-1", newer) != "" {
		t.Fatal("seed must drop a stale owner even without deleteExtract")
	}

	if unpack.lookupHookMessage("Show", "discord-1", old) != "" {
		t.Fatal("old extract must not keep the replaced cache")
	}
}
