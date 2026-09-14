package unpackerr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/xtractr"
)

func TestStatusPageIncludesResizableColumns(t *testing.T) {
	t.Parallel()

	for _, fragment := range []string{
		`id="items-table"`,
		`id="reset-columns"`,
		`class="column-resizer"`,
		`role="separator"`,
		`columnWidthStorageKey`,
		`localStorage.setItem`,
		`pointerdown`,
		`ArrowRight`,
		`id="auth-form"`,
		`api/auth/login`,
		`PBKDF2`,
		`iterations: 210000`,
		`response.status === 401`,
		`Sign in to Unpackerr`,
		`https://github.com/Unpackerr/unpackerr`,
		`stats[key]`,
	} {
		if !strings.Contains(statusPageHTML, fragment) {
			t.Errorf("status page does not contain column resizing fragment %q", fragment)
		}
	}

	for _, leak := range []string{"UnpackUI", "TheBadFella", "jsdelivr.net"} {
		if strings.Contains(statusPageHTML, leak) {
			t.Errorf("status page still contains fork leftover %q", leak)
		}
	}
}

type webStatusAPITestResponse struct {
	CompletedCount int                        `json:"completedCount"`
	Items          []webStatusAPITestItem     `json:"items"`
	Stats          map[string]json.RawMessage `json:"stats"`
}

type webStatusAPITestItem struct {
	Completed bool `json:"completed"`
}

func TestWebServerUIRoutes(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.Webserver.UI = true
	unpackerr.Webserver.URLBase = "/"
	unpackerr.Webserver.router = http.NewServeMux()
	unpackerr.webRoutes()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	unpackerr.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "status-shell") {
		t.Fatal("expected web UI HTML to be served at root when UI is enabled")
	}

	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", rec.Header().Get("X-Frame-Options"))
	}
}

func TestWebServerRootDefaultWithoutUI(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.Webserver.UI = false
	unpackerr.Webserver.URLBase = "/"
	unpackerr.Webserver.router = http.NewServeMux()
	unpackerr.webRoutes()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	unpackerr.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if strings.TrimSpace(rec.Body.String()) != "Welcome!" {
		t.Fatalf("expected Welcome!, got %q", rec.Body.String())
	}

	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/status", nil)
	rec = httptest.NewRecorder()
	unpackerr.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/status when ui=false, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestBuildWebStateIncludesProgress(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.Map["Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  EXTRACTING,
		Updated: now.Add(-30 * time.Second),
		IDs:     map[string]any{"reason": "download complete"},
		XProg: &ExtractProgress{
			Archives:  3,
			Extracted: 1,
			StartedAt: now.Add(-10 * time.Second),
			UpdatedAt: now,
			Progress: &xtractr.Progress{
				Total:      200,
				Wrote:      50,
				Compressed: 200,
				Read:       50,
				XFile: &xtractr.XFile{
					FilePath: "/downloads/Example.Release/file.part01.rar",
				},
			},
		},
	}

	snapshot := unpackerr.buildWebState(now)
	if snapshot.Stats.Extracting != 1 {
		t.Fatalf("expected 1 extracting item, got %d", snapshot.Stats.Extracting)
	}

	if len(snapshot.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(snapshot.Items))
	}

	progress := snapshot.Items[0].Progress
	if progress == nil {
		t.Fatal("expected progress to be present")
	}

	if progress.ArchiveIndex != 2 || progress.ArchiveCount != 3 {
		t.Fatalf("unexpected archive progress: %+v", progress)
	}

	if progress.Percent != 25 {
		t.Fatalf("expected 25 percent, got %.0f", progress.Percent)
	}

	if progress.Speed == "" {
		t.Fatal("expected extraction speed to be present")
	}

	if progress.ETA != "30s" {
		t.Fatalf("expected 30s ETA, got %q", progress.ETA)
	}
}

func TestBuildWebStateUsesFriendlyFolderDisplayName(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.folders = &Folders{
		Folders: map[string]*Folder{
			"/downloads/Example.Release": {
				Updated: now,
				Status:  EXTRACTED,
				Config: &FolderConfig{
					DeleteAfter: &cnfg.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}
	unpackerr.Map["/downloads/Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  EXTRACTED,
		Updated: now,
		IDs:     map[string]any{"title": "/downloads/Example.Release"},
	}

	snapshot := unpackerr.buildWebState(now)
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(snapshot.Items))
	}

	if snapshot.Items[0].Name != "Example.Release" {
		t.Fatalf("expected display name to use folder basename, got %q", snapshot.Items[0].Name)
	}

	if snapshot.Items[0].Path != "/downloads/Example.Release" {
		t.Fatalf("expected full path to remain unchanged, got %q", snapshot.Items[0].Path)
	}

	if snapshot.Items[0].StatusText != "Extracted" {
		t.Fatalf("expected folder extract status text to be simplified, got %q", snapshot.Items[0].StatusText)
	}

	if snapshot.Items[0].DeleteIn != "5m0s" {
		t.Fatalf("expected folder delete countdown, got %q", snapshot.Items[0].DeleteIn)
	}
}

func TestBuildWebStatusDetailsFiltersMarkerFiles(t *testing.T) {
	t.Parallel()

	item := &Extract{
		App:  FolderString,
		Path: "/downloads/Example.Release",
		IDs:  map[string]any{"title": "/downloads/Example.Release"},
		Resp: &xtractr.Response{
			NewFiles: []string{
				"/downloads/Example.Release/episode.mkv",
				"/downloads/Example.Release/_unpackerred.Example.Release.txt",
				"/downloads/Example.Release/subtitles.srt",
			},
		},
	}

	details := buildWebStatusDetails(item)
	if details == nil {
		t.Fatal("expected details to be present")
	}

	if details.Title != "Example.Release" {
		t.Fatalf("expected details title to use basename, got %q", details.Title)
	}

	if len(details.Files) != 2 {
		t.Fatalf("expected marker file to be filtered out, got %d files", len(details.Files))
	}
}

func TestBuildWebStateIncludesDeleteCountdownForImportedItems(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.Map["Example.Release"] = &Extract{
		App:         "Sonarr",
		Path:        "/downloads/Example.Release",
		Status:      IMPORTED,
		Updated:     now,
		DeleteDelay: 10 * time.Minute,
		IDs:         map[string]any{"title": "Example Release"},
	}

	snapshot := unpackerr.buildWebState(now.Add(90 * time.Second))
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(snapshot.Items))
	}

	if snapshot.Items[0].DeleteIn != "8m30s" {
		t.Fatalf("expected imported item delete countdown, got %q", snapshot.Items[0].DeleteIn)
	}
}

func TestBuildWebStateDoesNotDuplicateCompletedItemsAcrossRefreshes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 21, 53, 46, 0, time.UTC)
	unpackerr := New()
	unpackerr.folders = &Folders{
		Folders: map[string]*Folder{
			"/downloads/sample-large-zip-file.zip": {
				Updated: now,
				Status:  EXTRACTED,
				Config: &FolderConfig{
					DeleteAfter: &cnfg.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}
	unpackerr.Map["/downloads/sample-large-zip-file.zip"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/sample-large-zip-file.zip",
		Status:  EXTRACTED,
		Updated: now,
		IDs:     map[string]any{"title": "/downloads/sample-large-zip-file.zip"},
	}

	unpackerr.refreshWebState(now.Add(5 * time.Second))
	snapshot := unpackerr.buildWebState(now.Add(10 * time.Second))

	if len(snapshot.Items) != 1 {
		t.Fatalf("expected exactly one completed item after repeated refreshes, got %d", len(snapshot.Items))
	}

	if snapshot.CompletedCount != 1 {
		t.Fatalf("expected one completed item, got %d", snapshot.CompletedCount)
	}
}

func TestBuildWebStateReplacesCompletedItemWhenStatusChanges(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 21, 53, 46, 0, time.UTC)
	unpackerr := New()
	path := "/downloads/sample-large-zip-file.zip"
	unpackerr.Map[path] = &Extract{
		App:     FolderString,
		Path:    path,
		Status:  EXTRACTED,
		Updated: now,
		IDs:     map[string]any{"title": path},
	}

	unpackerr.refreshWebState(now)
	unpackerr.Map[path].Status = DELETED
	unpackerr.Map[path].Updated = now.Add(time.Minute)

	snapshot := unpackerr.buildWebState(now.Add(65 * time.Second))
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one logical item after status change, got %d", len(snapshot.Items))
	}

	if snapshot.Items[0].Status != DELETED.String() {
		t.Fatalf("expected retained item status to update to deleted, got %q", snapshot.Items[0].Status)
	}
}

func TestWebStatusAPI(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.refreshWebState(time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	unpackerr.webStatusAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var snapshot webStatusAPITestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("failed to decode status payload: %v", err)
	}

	if snapshot.Stats == nil {
		t.Fatal("expected stats in payload")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("raw decode: %v", err)
	}

	var stats map[string]json.RawMessage
	if err := json.Unmarshal(raw["stats"], &stats); err != nil {
		t.Fatalf("stats decode: %v", err)
	}

	if _, ok := stats["extracting"]; !ok {
		t.Fatalf("expected lowercase extracting key, got %v", stats)
	}

	if _, ok := stats["Extracting"]; ok {
		t.Fatal("did not expect PascalCase Extracting key")
	}
}

func TestBuildWebStateKeepsCompletedItemsAfterRemoval(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.Map["Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  DELETED,
		Updated: now,
		IDs:     map[string]any{"title": "Example Release"},
	}

	unpackerr.refreshWebState(now)
	delete(unpackerr.Map, "Example.Release")

	snapshot := unpackerr.buildWebState(now.Add(45 * time.Second))
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected completed item to remain in snapshot, got %d items", len(snapshot.Items))
	}

	if !snapshot.Items[0].Completed {
		t.Fatal("expected persisted item to be marked completed")
	}

	if snapshot.CompletedCount != 1 || snapshot.ActiveCount != 0 {
		t.Fatalf("unexpected counts: active=%d completed=%d", snapshot.ActiveCount, snapshot.CompletedCount)
	}
}

func TestWebClearCompletedAPI(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.Map["Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  DELETED,
		Updated: now,
		IDs:     map[string]any{"title": "Example Release"},
	}
	unpackerr.refreshWebState(now)
	delete(unpackerr.Map, "Example.Release")
	unpackerr.refreshWebState(now.Add(time.Minute))

	snapshot := unpackerr.clearCompletedWebItems(now.Add(2 * time.Minute))

	if snapshot.CompletedCount != 0 {
		t.Fatalf("expected completed items to be cleared, got %d", snapshot.CompletedCount)
	}

	if len(snapshot.Items) != 0 {
		t.Fatalf("expected no items after clear, got %d", len(snapshot.Items))
	}
}

func TestWebClearCompletedAPIHandlerUsesMainLoop(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	go unpackerr.runMainTasks(t.Context())

	unpackerr.Map["Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  DELETED,
		Updated: now,
		IDs:     map[string]any{"title": "Example Release"},
	}
	unpackerr.refreshWebState(now)
	delete(unpackerr.Map, "Example.Release")
	unpackerr.refreshWebState(now.Add(time.Minute))

	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/api/status/clear-completed", nil,
	)
	rec := httptest.NewRecorder()
	unpackerr.webClearCompletedAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %q", rec.Code, rec.Body.String())
	}

	var snapshot webStatusAPITestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("failed to decode clear payload: %v", err)
	}

	if snapshot.CompletedCount != 0 {
		t.Fatalf("expected completed items to be cleared, got %d", snapshot.CompletedCount)
	}
}

func TestBuildWebStateShowsNewCompletionAfterClear(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 12, 12, 0, 0, 0, time.UTC)
	unpackerr := New()
	unpackerr.Map["Example.Release"] = &Extract{
		App:     FolderString,
		Path:    "/downloads/Example.Release",
		Status:  DELETED,
		Updated: now,
		IDs:     map[string]any{"title": "Example Release"},
	}
	unpackerr.refreshWebState(now)
	unpackerr.clearCompletedWebItems(now.Add(time.Minute))

	unpackerr.Map["Example.Release"].Status = EXTRACTING
	unpackerr.Map["Example.Release"].Updated = now.Add(2 * time.Minute)
	unpackerr.refreshWebState(now.Add(2 * time.Minute))

	unpackerr.Map["Example.Release"].Status = EXTRACTED
	unpackerr.Map["Example.Release"].Updated = now.Add(3 * time.Minute)
	snapshot := unpackerr.buildWebState(now.Add(3 * time.Minute))

	if snapshot.CompletedCount != 1 {
		t.Fatalf("expected the new completion to show after clear, got completed=%d items=%d",
			snapshot.CompletedCount, len(snapshot.Items))
	}
}

func TestWebStatusRoutesRequireAuth(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.Webserver.UI = true
	unpackerr.Webserver.URLBase = "/"
	unpackerr.Webserver.router = http.NewServeMux()
	unpackerr.webRoutes()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	unpackerr.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/status without auth: %d %q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/status/clear-completed", nil)
	rec = httptest.NewRecorder()
	unpackerr.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/status/clear-completed without auth: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSkipWebAccessLogOnlyStatusWhenUIEnabled(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.Webserver.UI = true
	unpackerr.Webserver.URLBase = "/"

	var withLog, withoutLog int
	handler := unpackerr.skipWebAccessLog(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { withLog++ }),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { withoutLog++ }),
	)

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/status", nil))
	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/stats", nil))
	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

	if withoutLog != 1 {
		t.Fatalf("expected only GET /api/status to skip the access log, got %d", withoutLog)
	}

	if withLog != 2 {
		t.Fatalf("expected /api/stats and / to be logged, got %d", withLog)
	}
}

func TestSkipWebAccessLogDisabledWhenUIOff(t *testing.T) {
	t.Parallel()

	unpackerr := New()
	unpackerr.Webserver.UI = false
	unpackerr.Webserver.URLBase = "/"

	var withLog, withoutLog int
	handler := unpackerr.skipWebAccessLog(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { withLog++ }),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { withoutLog++ }),
	)

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/status", nil))
	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/stats", nil))

	if withoutLog != 0 || withLog != 2 {
		t.Fatalf("ui=false must not skip access logs: with=%d without=%d", withLog, withoutLog)
	}
}

func TestWebserverRestartRequiredIncludesUI(t *testing.T) {
	t.Parallel()

	cur := &WebServer{ListenAddr: "127.0.0.1:5656"}
	next := cloneWebserver(cur)
	next.UI = true

	if !webserverRestartRequired(cur, next) {
		t.Fatal("changing ui must require a restart")
	}

	if webserverRestartRequired(cur, cloneWebserver(cur)) {
		t.Fatal("unchanged webserver must not require a restart")
	}
}
