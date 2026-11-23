package unpackerr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && os.Args[1] == "-unpackerr-gate" {
		code, err := strconv.Atoi(os.Args[2])
		if err != nil {
			os.Exit(2)
		}

		if code < 0 {
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}

		os.Exit(code)
	}

	os.Exit(m.Run())
}

func gateCommand(t *testing.T, code int) string {
	t.Helper()

	bin := os.Args[0]
	if strings.Contains(bin, " ") {
		t.Fatal("test binary path contains a space")
	}

	return fmt.Sprintf("%s -unpackerr-gate %d", bin, code)
}

func gateHook(t *testing.T, name string, code int, timeout time.Duration) *WebhookConfig {
	t.Helper()

	return &WebhookConfig{
		Name:    name,
		Command: gateCommand(t, code),
		Gate:    true,
		Timeout: cnfg.Duration{Duration: timeout},
	}
}

func TestCommandGateExitCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   int
		status ExtractStatus
		note   string
	}{
		{name: "ready", code: 0, status: QUEUED},
		{name: "not yet", code: 75, status: WAITING, note: noteWaitingGate},
		{name: "no opinion", code: 2, status: QUEUED},
		{name: "timeout", code: -1, status: QUEUED},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			timeout := time.Second
			if one.code < 0 {
				timeout = 200 * time.Millisecond
			}

			unpack, item := newSyncthingTestItem(t, 100)
			unpack.Cmdhook = instanceMap([]*WebhookConfig{gateHook(t, "gate", one.code, timeout)})

			unpack.extractCompletedDownload("Show.Name", time.Now(), item)

			if item.Status != one.status || item.Note != one.note {
				t.Fatalf("status %s note %q, want %s %q", item.Status, item.Note, one.status, one.note)
			}
		})
	}
}

func TestCommandGateRunsAfterDiskGate(t *testing.T) {
	t.Parallel()

	// show.rar is 100 bytes. A ready hook must not skip the short-download wait.
	unpack, item := newSyncthingTestItem(t, 300)
	unpack.Cmdhook = instanceMap([]*WebhookConfig{gateHook(t, "gate", 0, time.Second)})

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingSyncthing {
		t.Fatalf("short download: status %s note %q, want waiting for syncthing", item.Status, item.Note)
	}
}

func TestCommandGateNotYetKeepsExtractedRescan(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 100)
	item.Status = EXTRACTED
	item.Resp = &xtractr.Response{
		Done:     true,
		NewFiles: []string{filepath.Join(item.Path, "subs.srt")},
		Archives: xtractr.ArchiveList{item.Path: {filepath.Join(item.Path, "subs.zip")}},
		Size:     1,
	}
	unpack.Cmdhook = instanceMap([]*WebhookConfig{gateHook(t, "gate", 75, time.Second)})

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != EXTRACTED || item.Note != noteWaitingGate {
		t.Fatalf("rescan held by the hook: status %s note %q", item.Status, item.Note)
	}
}

func TestCommandGateAnyNotReadyHolds(t *testing.T) {
	t.Parallel()

	unpack, item := newSyncthingTestItem(t, 100)
	unpack.Cmdhook = instanceMap([]*WebhookConfig{
		gateHook(t, "ready", 0, time.Second),
		gateHook(t, "wait", 75, time.Second),
	})

	unpack.extractCompletedDownload("Show.Name", time.Now(), item)

	if item.Status != WAITING || item.Note != noteWaitingGate {
		t.Fatalf("one hook not ready: status %s note %q", item.Status, item.Note)
	}
}

func TestGateHookIsNotAnEventHook(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Cmdhook = instanceMap([]*WebhookConfig{
		gateHook(t, "gate", 0, time.Second),
		{
			Name:    "notify",
			Command: gateCommand(t, 0),
			Events:  ExtractStatuses{EXTRACTED},
			Timeout: cnfg.Duration{Duration: time.Second},
		},
	})

	item := &Extract{App: starr.Sonarr, Status: EXTRACTED, Path: t.TempDir(), Updated: time.Now()}
	unpack.runAllHooks("Show.Name", item, item)

	if unpack.hookWorker.Len() != 1 {
		t.Fatalf("event queue: %d, want the notify hook only", unpack.hookWorker.Len())
	}
}
