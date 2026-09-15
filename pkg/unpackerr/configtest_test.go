package unpackerr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"github.com/Unpackerr/unpackerr/pkg/hooks"
	"golift.io/starr"
)

const fakeStarrQueueJSON = `{
  "totalRecords": 3,
  "records": [
    {"title": "Show.S01E01", "protocol": "torrent"},
    {"title": "Show.S01E02", "protocol": "usenet"},
    {"title": "Show.S01E03", "protocol": "unknown"}
  ]
}`

func TestConfigTestUnauthorized(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", `{}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestRequiresWrite(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	readKey := strings.Repeat("R", apiKeyMinLen)
	writeKey := strings.Repeat("W", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"sonarrr": {Permissions: []string{PermReadConfig(SectionSonarr)}},
		"sonarrw": {Permissions: []string{PermWriteConfig(SectionSonarr)}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys,
		APIKey{Name: "read", Key: readKey, Roles: []string{"sonarrr"}},
		APIKey{Name: "write", Key: writeKey, Roles: []string{"sonarrw"}},
	)

	body := `{"url":"http://127.0.0.1:8989"}`
	if rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", body, func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("read %d %s", rec.Code, rec.Body.String())
	}

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", body, func(req *http.Request) {
		req.Header.Set(headerAPIKey, writeKey)
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("write %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestUntestableSection(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/general/test", `{}`, putKey(unpack))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), errSectionNotTestable.Error()) {
		t.Fatalf("general %d %s", rec.Code, rec.Body.String())
	}

	rec = doAuth(t, unpack, http.MethodPost, "/api/config/folders/test", `{}`, putKey(unpack))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("folders %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestStarrMissingAccess(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	key := putKey(unpack)
	starrKey := strings.Repeat("k", apiKeyMinLength)

	if rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", `{}`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty %d %s", rec.Code, rec.Body.String())
	}

	short := `{"url":"http://127.0.0.1:8989","apiKey":"short"}`

	if rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test",
		short, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("short key %d %s", rec.Code, rec.Body.String())
	}

	badURL := `{"url":"not-a-url","apiKey":"` + starrKey + `"}`

	if rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test",
		badURL, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad url %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestStarrQueue(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/queue") {
			http.NotFound(writer, request)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(fakeStarrQueueJSON))
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	starrKey := strings.Repeat("k", apiKeyMinLength)
	body := `{"url":"` + server.URL + `","apiKey":"` + starrKey + `","timeout":"5s"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", body, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}

	var got starrTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	if got.Queued != 3 || got.Retrieved != 3 || got.Torrents != 1 || got.Nzbs != 1 || got.Other != 1 {
		t.Fatalf("counts %+v", got)
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestStarrFillsFromLive(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(fakeStarrQueueJSON))
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	starrKey := strings.Repeat("s", apiKeyMinLength)
	unpack.Sonarr = InstanceMap[SonarrConfig]{
		"uhd": {URL: server.URL, APIKey: starrKey},
	}

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", `{"slug":"uhd"}`, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("live fill %d %s", rec.Code, rec.Body.String())
	}

	var got starrTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	if got.Torrents != 1 || got.Nzbs != 1 {
		t.Fatalf("counts %+v", got)
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestStarrRemoteError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "nope", http.StatusInternalServerError)
	}))
	url := server.URL
	server.Close()

	unpack := testAuthUnpackerr(t)
	body := `{"url":"` + url + `","apiKey":"` + strings.Repeat("k", apiKeyMinLength) + `","timeout":"1s"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/sonarr/test", body, putKey(unpack))
	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("remote %d %s", rec.Code, rec.Body.String())
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestWebhook(t *testing.T) {
	t.Parallel()

	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		gotBody = string(body)

		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	body := `{"url":"` + server.URL + `","event":"extracted","app":"radarr","timeout":"2s"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test", body, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook %d %s", rec.Code, rec.Body.String())
	}

	var got hookTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	if got.Status != "ok" {
		t.Fatalf("status %+v", got)
	}

	requireTestElapsed(t, rec.Body.Bytes())

	if !strings.Contains(gotBody, "extracted") || !strings.Contains(gotBody, "Radarr") {
		t.Fatalf("payload %s", gotBody)
	}
}

func TestOverlayHookKeepsLiveUnlessPosted(t *testing.T) {
	t.Parallel()

	live := &hooks.Config{
		URL:       "http://live.example/hook",
		TempName:  "discord",
		TmplPath:  "/tmp/hook.tmpl",
		Shell:     true,
		IgnoreSSL: true,
		Name:      "discord",
		Nickname:  "Unpackerr",
	}

	keep := hooks.CloneList([]*hooks.Config{live})[0]
	overlayHook(keep, configTestRequest{})

	if keep.URL != live.URL || keep.TempName != live.TempName || keep.TmplPath != live.TmplPath ||
		keep.Shell != live.Shell || keep.IgnoreSSL != live.IgnoreSSL || keep.Name != live.Name {
		t.Fatalf("empty overlay template=%q path=%q shell=%v ssl=%v",
			keep.TempName, keep.TmplPath, keep.Shell, keep.IgnoreSSL)
	}

	var omitted configTestRequest
	if err := json.Unmarshal([]byte(`{"url":"http://posted.example/hook"}`), &omitted); err != nil {
		t.Fatal(err)
	}

	posted := hooks.CloneList([]*hooks.Config{live})[0]
	overlayHook(posted, omitted)

	if posted.URL != "http://posted.example/hook" || posted.TempName != "discord" ||
		posted.TmplPath != "/tmp/hook.tmpl" || !posted.Shell || !posted.IgnoreSSL {
		t.Fatalf("omitted fields template=%q shell=%v ssl=%v url=%q",
			posted.TempName, posted.Shell, posted.IgnoreSSL, posted.URL)
	}

	var off configTestRequest
	if err := json.Unmarshal([]byte(
		`{"template":"gotify","templatePath":"/tmp/other.tmpl","shell":false,"ignoreSsl":false}`,
	), &off); err != nil {
		t.Fatal(err)
	}

	forced := hooks.CloneList([]*hooks.Config{live})[0]
	overlayHook(forced, off)

	if forced.TempName != "gotify" || forced.TmplPath != "/tmp/other.tmpl" ||
		forced.Shell || forced.IgnoreSSL || forced.URL != live.URL {
		t.Fatalf("posted override template=%q path=%q shell=%v ssl=%v",
			forced.TempName, forced.TmplPath, forced.Shell, forced.IgnoreSSL)
	}
}

func TestConfigTestWebhookFillsFromLive(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("pong"))
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	unpack.Webhook = InstanceMap[WebhookConfig]{
		"discord": {Name: "discord", URL: server.URL},
	}

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test",
		`{"slug":"discord","event":"queued","app":"folder"}`, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("live hook %d %s", rec.Code, rec.Body.String())
	}

	var got hookTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	if got.Reply != "pong" {
		t.Fatalf("reply %+v", got)
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestWebhookKeepsLiveTemplate(t *testing.T) {
	t.Parallel()

	var gotBody string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		gotBody = string(body)

		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	unpack.Webhook = InstanceMap[WebhookConfig]{
		"discord": {Name: "discord", URL: server.URL, TempName: "discord"},
	}

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test",
		`{"slug":"discord","event":"queued"}`, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("live template %d %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(gotBody, `"embeds"`) {
		t.Fatalf("expected discord template, got %s", gotBody)
	}

	if strings.Contains(gotBody, "unpackerr_eventtype") {
		t.Fatalf("default notifiarr template leaked: %s", gotBody)
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestWebhookRemoteError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	unpack := testAuthUnpackerr(t)
	body := `{"url":"` + server.URL + `"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test", body, putKey(unpack))
	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("424 %d %s", rec.Code, rec.Body.String())
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestWebhookNoURL(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test",
		`{"event":"extracted"}`, putKey(unpack))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no url %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestWebhookUnknownEvent(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	body := `{"url":"http://127.0.0.1/hook","event":"nope"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/webhooks/test", body, putKey(unpack))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("event %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigTestCmdhook(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("no /bin/echo")
	}

	unpack := testAuthUnpackerr(t)
	body := `{"command":"/bin/echo","event":"waiting","timeout":"2s"}`

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/cmdhooks/test", body, putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("cmdhook %d %s", rec.Code, rec.Body.String())
	}

	requireTestElapsed(t, rec.Body.Bytes())
}

func TestConfigTestCmdhookNoCommand(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodPost, "/api/config/cmdhooks/test", `{}`, putKey(unpack))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no cmd %d %s", rec.Code, rec.Body.String())
	}
}

func TestClampTestTimeout(t *testing.T) {
	t.Parallel()

	if got := clampTestTimeout(0, 0, 0); got != defaultTestTO {
		t.Fatalf("default %v", got)
	}

	if got := clampTestTimeout(0, 3*time.Second, time.Minute); got != 3*time.Second {
		t.Fatalf("live %v", got)
	}

	if got := clampTestTimeout(2*time.Hour, 0, 0); got != maxTestTimeout {
		t.Fatalf("cap %v", got)
	}
}

func TestClipReply(t *testing.T) {
	t.Parallel()

	if got := clipReply("ok"); got != "ok" {
		t.Fatalf("short %q", got)
	}

	long := strings.Repeat("é", maxHookReply+3)

	got := clipReply(long)
	if utf8.RuneCountInString(got) != maxHookReply+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long %d %q", utf8.RuneCountInString(got), got[len(got)-4:])
	}
}

func requireTestElapsed(t *testing.T, body []byte) {
	t.Helper()

	var got struct {
		Elapsed string `json:"elapsed"`
	}

	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}

	if got.Elapsed == "" {
		t.Fatalf("missing elapsed %s", body)
	}

	if _, err := time.ParseDuration(got.Elapsed); err != nil {
		t.Fatalf("elapsed %q: %v", got.Elapsed, err)
	}
}

func TestParseTestEventAndApp(t *testing.T) {
	t.Parallel()

	event, err := parseTestEvent("")
	if err != nil || event != extract.EXTRACTED {
		t.Fatalf("default %v %v", event, err)
	}

	event, err = parseTestEvent("queued")
	if err != nil || event != extract.QUEUED {
		t.Fatalf("queued %v %v", event, err)
	}

	if _, err = parseTestEvent("nope"); err == nil {
		t.Fatal("expected unknown event")
	}

	if got := testHookApp(""); got != starr.Sonarr {
		t.Fatalf("default app %q", got)
	}

	if got := testHookApp("folder"); got != FolderString {
		t.Fatalf("folder %q", got)
	}

	if got := testHookApp("UHD"); got != starr.App("UHD") {
		t.Fatalf("named %q", got)
	}
}
