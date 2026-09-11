package hooks

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/cnfg"
)

type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}
func (noopLogger) Errorf(string, ...any) {}
func (noopLogger) Debugf(string, ...any) {}

func TestWorkerFIFOAndAfter(t *testing.T) {
	t.Parallel()

	orderFile := filepath.Join(t.TempDir(), "order")
	if err := os.WriteFile(orderFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	webhook := mustWebhook(t, recordingServer(t, orderFile).URL, time.Second)
	command := mustCmdhook(t, recordCommand(t, orderFile, "command"), 5*time.Second)
	payload := &Payload{Path: "/dl/album", Event: extract.EXTRACTED}
	after := drainWorker(t, &Item{Config: webhook, Payload: payload}, &Item{Config: command, Payload: payload})

	if after != 2 {
		t.Fatalf("after callbacks: got %d want 2", after)
	}

	got := strings.Fields(string(readFile(t, orderFile)))
	if strings.Join(got, " ") != "webhook command" {
		t.Fatalf("delivery order: got %v want [webhook command]", got)
	}
}

func TestWorkerAfterOnFailedWebhook(t *testing.T) {
	t.Parallel()

	hook := mustWebhook(t, "http://127.0.0.1:1", 200*time.Millisecond)
	after := drainWorker(t, &Item{Config: hook, Payload: &Payload{Path: "/dl/fail", Event: extract.EXTRACTFAILED}})

	if after != 1 {
		t.Fatalf("after callbacks: got %d want 1", after)
	}

	if posts, fails := hook.Counts(); posts != 1 || fails != 1 {
		t.Fatalf("webhook counts: posts=%d fails=%d want 1/1", posts, fails)
	}
}

func drainWorker(t *testing.T, items ...*Item) int32 {
	t.Helper()

	worker := NewWorker(len(items) + 1)

	var afterCount atomic.Int32

	done := make(chan struct{})

	go func() {
		worker.Run(noopLogger{}, func() { afterCount.Add(1) })
		close(done)
	}()

	for _, item := range items {
		worker.Enqueue(item)
	}

	close(worker.queue)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not finish queued items")
	}

	return afterCount.Load()
}

func recordingServer(t *testing.T, orderFile string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		file, err := os.OpenFile(orderFile, os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = file.WriteString("webhook\n")
			_ = file.Close()
		}

		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func mustWebhook(t *testing.T, url string, timeout time.Duration) *Config {
	t.Helper()

	list := []*Config{{Name: "webhook", URL: url, Silent: true, Timeout: cnfg.Duration{Duration: timeout}}}
	if err := ValidateWebhooks(list, timeout); err != nil {
		t.Fatal(err)
	}

	return list[0]
}

func mustCmdhook(t *testing.T, command string, timeout time.Duration) *Config {
	t.Helper()

	list := []*Config{{Name: "command", Command: command, Silent: true, Timeout: cnfg.Duration{Duration: timeout}}}
	if err := ValidateCmdhooks(list, timeout, nil); err != nil {
		t.Fatal(err)
	}

	return list[0]
}

func recordCommand(t *testing.T, orderPath, token string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "record.sh")
	script := "#!/bin/sh\nprintf '%s\\n' '" + token + "' >> '" + orderPath + "'\n"

	if runtime.GOOS == "windows" {
		path = filepath.Join(t.TempDir(), "record.cmd")
		script = "@echo off\r\n>>\"" + orderPath + "\" echo " + token + "\r\n"
	}

	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		return "/bin/sh " + path
	}

	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
