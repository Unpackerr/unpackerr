package frontend

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexHandlerServesHTML(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	IndexHandler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "Unpackerr") {
		t.Fatalf("body %q", rec.Body.String())
	}

	if _, err := fs.Stat(root, "index.html"); err == nil && !strings.Contains(rec.Body.String(), "assets/") {
		t.Fatal("built index must reference assets/")
	}
}

func TestIndexHandlerMissingAssetIs404(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	IndexHandler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/missing.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset %d %q", rec.Code, rec.Body.String())
	}
}
