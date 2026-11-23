package unpackerr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/xtractr"
)

// gateHelperFlag makes this test binary act as a gate command.
const gateHelperFlag = "-unpackerr-gate"

func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && os.Args[1] == gateHelperFlag {
		os.Exit(gateHelper(os.Args[2:]))
	}

	os.Exit(m.Run())
}

// gateHelper is the command the gate tests run. It prints two lines and exits
// with the code it is given. A negative code sleeps well past any timeout the
// tests use. With "child" it first leaves a process behind that writes to the
// same output.
func gateHelper(args []string) int {
	if args[0] == "leftover" {
		return gateLeftover()
	}

	code, err := strconv.Atoi(args[0])
	if err != nil {
		return 2
	}

	fmt.Fprintf(os.Stdout, "\ngate says %d\nsecond line\n", code)

	if len(args) > 1 && args[1] == "child" {
		child := exec.CommandContext(context.Background(), os.Args[0], gateHelperFlag, "leftover") //nolint:gosec
		child.Stdout, child.Stderr = os.Stdout, os.Stderr

		if err := child.Start(); err != nil {
			return 2
		}
	}

	if code < 0 {
		time.Sleep(gateHelperSleep)
		return 0
	}

	return code
}

// gateLeftover is the process a gate command leaves behind. It holds the
// command's output open until the test that started it is over: a command
// hook is given the download path, and the test removes that folder.
func gateLeftover() int {
	path := os.Getenv("UN_PATH")
	if path == "" {
		return 2
	}

	for end := time.Now().Add(gateLeftoverLife); time.Now().Before(end); time.Sleep(gateLeftoverPoll) {
		if _, err := os.Stat(path); err != nil { //nolint:gosec // the test's own temp folder.
			break
		}
	}

	return 0
}

const (
	gateHelperSleep  = 5 * time.Second  // how long a sleeping gate command runs.
	gateLeftoverLife = 30 * time.Second // the longest a leftover process holds the output open.
	gateLeftoverPoll = 50 * time.Millisecond
)

func gateCommand(t *testing.T, code int) string {
	t.Helper()

	return fmt.Sprintf("%s %s %d", gateBinary(t), gateHelperFlag, code)
}

// gateBinary is this test binary, the way a command hook can name it. A
// command is split on spaces, so a path with one in it does not work. The
// path from the working directory often has none when the full path does: a
// space in a user name sits above both. Without either, the test is skipped.
func gateBinary(t *testing.T) string {
	t.Helper()

	bin := os.Args[0]
	if !strings.ContainsAny(bin, " \t") {
		return bin
	}

	if dir, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(dir, bin); err == nil && !strings.ContainsAny(rel, " \t") {
			return rel
		}
	}

	t.Skip("gate tests need a path to the test binary without spaces")

	return ""
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
	item.Unpacked = []string{filepath.Join(item.Path, "subs.zip")}
	item.Resp = &xtractr.Response{
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

// A gate command can leave a process behind that keeps its output open. The
// main loop does not wait for that process: the command's own exit decides,
// at the hook timeout at the latest.
func TestCommandGateDoesNotWaitForLeftoverProcess(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   int
		status ExtractStatus
		note   string
	}{
		{name: "ready", code: 0, status: QUEUED},
		{name: "not yet", code: 75, status: WAITING, note: noteWaitingGate},
		{name: "timeout", code: -1, status: QUEUED},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			// A command that ends by itself gets the time a loaded machine needs to start it.
			timeout := gateLeftoverLife
			if one.code < 0 {
				timeout = 500 * time.Millisecond
			}

			hook := gateHook(t, "gate", one.code, timeout)
			hook.Command += " child"

			var errs bytes.Buffer

			unpack, item := newSyncthingTestItem(t, 100)
			unpack.Error.SetOutput(&errs)
			unpack.Cmdhook = instanceMap([]*WebhookConfig{hook})

			start := time.Now()
			unpack.extractCompletedDownload("Show.Name", start, item)

			if item.Status != one.status || item.Note != one.note {
				t.Fatalf("status %s note %q, want %s %q", item.Status, item.Note, one.status, one.note)
			}

			// Only the command that ran out of time failed.
			if failed := errs.Len() != 0; failed != (one.code < 0) {
				t.Fatalf("error log %q", errs.String())
			}

			// The leftover process holds the output open until this test is over.
			if elapsed := time.Since(start); elapsed >= gateLeftoverLife/2 {
				t.Fatalf("the gate took %v, it waited for the leftover process", elapsed.Round(time.Millisecond))
			}
		})
	}
}

// The first line a gate command prints says why it holds the download, or
// why it failed. A silent hook keeps its output out of the log.
func TestCommandGateLogsFirstLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		code   int
		silent bool
		want   string
	}{
		{name: "not yet", code: 75, want: "command hook gate is not ready: gate says 75\n"},
		{name: "not yet, silent", code: 75, silent: true, want: "command hook gate is not ready\n"},
		{name: "failed", code: 2, want: "exit status 2: gate says 2\n"},
		{name: "failed, silent", code: 2, silent: true, want: "exit status 2\n"},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			hook := gateHook(t, "gate", one.code, 10*time.Second)
			hook.Silent = one.silent

			unpack, item := newSyncthingTestItem(t, 100)
			unpack.Info.SetOutput(&logs)
			unpack.Error.SetOutput(&logs)
			unpack.Cmdhook = instanceMap([]*WebhookConfig{hook})

			unpack.commandGateWaiting("Show.Name", item)

			if got := logs.String(); !strings.Contains(got, one.want) || strings.Contains(got, "second line") {
				t.Fatalf("log %q, want it to hold %q and no second line", got, one.want)
			}
		})
	}
}
