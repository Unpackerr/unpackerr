package hooks

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/cnfg"
)

func TestFireWebhook(t *testing.T) {
	t.Parallel()

	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		gotBody = string(body)

		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	payload := SamplePayload()
	if err := PrepareSample(payload, extract.EXTRACTED); err != nil {
		t.Fatal(err)
	}

	payload.App = "Sonarr"

	hook := &Config{
		Name:    "test",
		URL:     server.URL,
		CType:   "application/json",
		Timeout: cnfg.Duration{Duration: 2 * time.Second},
	}

	if _, err := Fire(t.Context(), hook, payload); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(gotBody, `"unpackerr_eventtype": "extracted"`) &&
		!strings.Contains(gotBody, `"unpackerr_eventtype":"extracted"`) {
		t.Fatalf("payload %s", gotBody)
	}

	if !strings.Contains(gotBody, "Sonarr") {
		t.Fatalf("missing app in %s", gotBody)
	}
}

func echoCommand(t *testing.T) string {
	t.Helper()

	// runCmd splits the command with strings.Fields, so a path like
	// "C:\Program Files\Git\usr\bin\echo.exe" becomes "C:\Program".
	if runtime.GOOS == "windows" {
		path := filepath.Join(t.TempDir(), "echo.cmd")
		if strings.ContainsAny(path, " \t") {
			t.Skip("command hook tests need a temp path without spaces")
		}

		if err := os.WriteFile(path, []byte("@echo ok\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		return path
	}

	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found in PATH")
	}

	path := filepath.Join(t.TempDir(), "echo.sh")
	if strings.ContainsAny(shell, " \t") || strings.ContainsAny(path, " \t") {
		t.Skip("command hook tests need paths without spaces")
	}

	if err := os.WriteFile(path, []byte("printf '%s\\n' ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return shell + " " + path
}

func TestFireCmdhook(t *testing.T) {
	t.Parallel()

	payload := SamplePayload()
	if err := PrepareSample(payload, extract.QUEUED); err != nil {
		t.Fatal(err)
	}

	hook := &Config{
		Name:    "echo",
		Command: echoCommand(t),
		Timeout: cnfg.Duration{Duration: 2 * time.Second},
	}

	out, err := Fire(t.Context(), hook, payload)
	if err != nil {
		t.Fatal(err)
	}

	if out == "" {
		t.Fatal("expected command output or success")
	}
}

func TestFireNilAndEmpty(t *testing.T) {
	t.Parallel()

	if _, err := Fire(t.Context(), nil, SamplePayload()); !errors.Is(err, ErrNilConfig) {
		t.Fatalf("nil %v", err)
	}

	if _, err := Fire(t.Context(), &Config{}, SamplePayload()); !errors.Is(err, ErrWebhookNoURL) {
		t.Fatalf("empty %v", err)
	}
}

func TestPayloadCustomIDEnv(t *testing.T) {
	t.Parallel()

	env, err := cnfg.MarshalENV(&Payload{
		Path:      "/dl",
		IDs:       map[string]any{"title": "Show"},
		CustomIDs: map[string]string{"url": "https://unpackerr.example"},
	}, "UN")
	if err != nil {
		t.Fatal(err)
	}

	if env["UN_IDS_title"] != "Show" {
		t.Fatalf("ids env %+v", env)
	}

	if env["UN_CUSTOM_ID_url"] != "https://unpackerr.example" {
		t.Fatalf("custom id env %+v", env)
	}

	if _, ok := env["UN_IDS_url"]; ok {
		t.Fatalf("custom id merged into UN_IDS_ %+v", env)
	}
}
