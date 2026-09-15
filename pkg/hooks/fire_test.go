package hooks

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestFireCmdhook(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("no /bin/echo")
	}

	payload := SamplePayload()
	if err := PrepareSample(payload, extract.QUEUED); err != nil {
		t.Fatal(err)
	}

	hook := &Config{
		Name:    "echo",
		Command: "/bin/echo",
		Timeout: cnfg.Duration{Duration: 2 * time.Second},
	}

	out, err := Fire(t.Context(), hook, payload)
	if err != nil {
		t.Fatal(err)
	}

	if out == "" && err != nil {
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
