package unpackerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golift.io/cnfg"
	"golift.io/starr/sonarr"
)

func TestConfigPutGeneralRoundTrip(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 200
	unpack.Items = make([]string, trayHistory)
	unpack.Items[0] = "queued"
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	general.KeepHistory = 50
	general.Debug = true

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.KeepHistory != 50 || !unpack.Config.Debug {
		t.Fatalf("applied %+v debug %v", unpack.KeepHistory, unpack.Config.Debug)
	}

	if len(unpack.Items) != trayHistory || unpack.Items[0] != "queued" {
		t.Fatalf("PUT resized history items: %+v", unpack.Items)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if !strings.Contains(text, "keep_history = 50") || !strings.Contains(text, "debug = true") {
		t.Fatalf("fileConfig write missed PUT:\n%s", text)
	}
}

// enableHistoryPUT turns keep_history on through the API, the way the settings
// UI will, on an instance that started with it disabled.
func enableHistoryPUT(t *testing.T, unpack *Unpackerr, keep uint) {
	t.Helper()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	general.KeepHistory = keep

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.KeepHistory != keep {
		t.Fatalf("applied keep_history %d", unpack.KeepHistory)
	}
}

func TestConfigPutGeneralEnablesTrayHistory(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 0
	unpack.snapshotFileConfig()

	enableHistoryPUT(t, unpack, 50)

	if len(unpack.Items) != trayHistory {
		t.Fatalf("tray items after enabling history: %d", len(unpack.Items))
	}

	unpack.updateHistory("queued")

	if unpack.Items[0] != "queued" {
		t.Fatalf("updateHistory after enable: %+v", unpack.Items)
	}
}

// Enabling history at runtime also has to resolve the JSONL path, or rows would
// stay in memory and vanish on the next restart.
func TestConfigPutGeneralEnablesHistoryFile(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 0
	unpack.snapshotFileConfig()

	if unpack.histPath != "" {
		t.Fatal("history path should be unset while keep_history is 0")
	}

	enableHistoryPUT(t, unpack, 50)

	if unpack.histPath == "" {
		t.Fatal("enabling keep_history did not resolve the history file path")
	}

	unpack.maybeRecordHistory("/dl/done", &Extract{Path: "/dl/done", Status: IMPORTED, Updated: time.Now()})

	written, err := os.ReadFile(unpack.histPath)
	if err != nil {
		t.Fatalf("history file after enable: %v", err)
	}

	if !strings.Contains(string(written), `"id":"/dl/done"`) {
		t.Fatalf("record was not persisted:\n%s", written)
	}
}

func TestConfigPutCmdhookRejectsWhitespaceCommand(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodPut, "/api/config/cmdhooks", `[{"command":"   "}]`, withKey)
	if got.Code != http.StatusBadRequest {
		t.Fatalf("whitespace cmdhook %d %s", got.Code, got.Body.String())
	}
}

func TestConfigPutWebserverRejectsFileKeyCollision(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	fileKey, liveKey := splitFileAndLiveAdminKeys(t, unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, liveKey)
	})
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	for idx := range web.APIKeys {
		web.APIKeys[idx].Key = ""
	}

	web.APIKeys = append(web.APIKeys, APIKey{
		Name:  "extra",
		Key:   fileKey,
		Roles: []string{RoleAdmin},
	})

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), func(req *http.Request) {
		req.Header.Set(headerAPIKey, liveKey)
	})
	if put.Code != http.StatusBadRequest {
		t.Fatalf("file key collision %d %s", put.Code, put.Body.String())
	}

	if unpack.Webserver.adminAPIKey() != liveKey {
		t.Fatal("rejected PUT mutated live key")
	}

	if unpack.fileConfig.Webserver.APIKeys[0].Key != fileKey {
		t.Fatalf("rejected PUT mutated file key %q", unpack.fileConfig.Webserver.APIKeys[0].Key)
	}
}

func TestConfigPutSonarrValidates(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	bad := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr",
		`[{"url":"not-a-url","apiKey":"`+strings.Repeat("k", apiKeyMinLength)+`"}]`, withKey)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad url %d %s", bad.Code, bad.Body.String())
	}

	good := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr",
		`[{"url":"http://127.0.0.1:8989","apiKey":"`+strings.Repeat("k", apiKeyMinLength)+`","path":"/dl"}]`, withKey)
	if good.Code != http.StatusOK {
		t.Fatalf("good %d %s", good.Code, good.Body.String())
	}

	if len(unpack.Sonarr) != 1 || unpack.Sonarr["0"].URL != "http://127.0.0.1:8989" {
		t.Fatalf("sonarr %+v", unpack.Sonarr)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), "http://127.0.0.1:8989") {
		t.Fatalf("sonarr PUT missed fileConfig:\n%s", written)
	}

	if !strings.Contains(string(written), "[sonarr.0]") {
		t.Fatalf("sonarr PUT should write named tables:\n%s", written)
	}

	if strings.Contains(string(written), "[[sonarr]]") {
		t.Fatalf("sonarr PUT wrote a 0.x array table:\n%s", written)
	}
}

func TestConfigPutLidarrURLNeedsAPIKey(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	emptyKey := doAuth(t, unpack, http.MethodPut, "/api/config/lidarr",
		`[{"url":"http://sdfsdf.sdsd.com/lidarr","apiKey":""}]`, withKey)
	if emptyKey.Code != http.StatusBadRequest {
		t.Fatalf("empty key %d %s", emptyKey.Code, emptyKey.Body.String())
	}

	if !strings.Contains(emptyKey.Body.String(), "API Key") {
		t.Fatalf("want API key error, got %s", emptyKey.Body.String())
	}

	if !strings.Contains(emptyKey.Body.String(), `\"0\"`) {
		t.Fatalf("want instance key in error, got %s", emptyKey.Body.String())
	}

	shortKey := doAuth(t, unpack, http.MethodPut, "/api/config/lidarr",
		`[{"url":"http://sdfsdf.sdsd.com/lidarr","apiKey":"tooshort"}]`, withKey)
	if shortKey.Code != http.StatusBadRequest {
		t.Fatalf("short key %d %s", shortKey.Code, shortKey.Body.String())
	}

	if !strings.Contains(shortKey.Body.String(), "key length") {
		t.Fatalf("want short-key error, got %s", shortKey.Body.String())
	}

	good := doAuth(t, unpack, http.MethodPut, "/api/config/lidarr",
		`[{"url":"http://sdfsdf.sdsd.com/lidarr","apiKey":"`+
			strings.Repeat("k", apiKeyMinLength)+`","split_flac":true}]`, withKey)
	if good.Code != http.StatusOK {
		t.Fatalf("lidarr %d %s", good.Code, good.Body.String())
	}
}

func TestConfigPutSonarrRejectsSplitFlac(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr",
		`[{"url":"http://127.0.0.1:8989","apiKey":"`+strings.Repeat("k", apiKeyMinLength)+`","split_flac":false}]`, withKey)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("split_flac %d %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "split_flac") {
		t.Fatalf("want unknown field, got %s", rec.Body.String())
	}
}

func TestConfigPutNeedsWritePerm(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	readKey := strings.Repeat("P", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"general": {Permissions: []string{PermReadConfig(SectionGeneral)}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "ro",
		Key:   readKey,
		Roles: []string{"general"},
	})

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general", `{}`, func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("readonly put %d", rec.Code)
	}
}

func TestConfigPutWebserverFilepathPassword(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	passFile := filepath.Join(dir, "ui.pass")

	if err := os.WriteFile(passFile, []byte("correct-horse\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(dir, "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	web.UIPassword = CryptPass(filePrefix + passFile)

	unpack.fileConfig.Webserver.UIPassword = web.UIPassword

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if !unpack.Webserver.UIPassword.ValidPlain(defaultUIUser, "correct-horse") {
		t.Fatal("live password must expand filepath:")
	}

	stored := unpack.fileConfig.Webserver.UIPassword.Val()
	if stored != filePrefix+passFile {
		t.Fatalf("file snapshot must keep filepath:, got %q", stored)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), `filepath:`) {
		t.Fatalf("config write dropped filepath:\n%s", written)
	}
}

func putKey(unpack *Unpackerr) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}
}

func TestConfigPutRejectsUnknownAndEmptyJSON(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 200
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general", `{}`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty object %d %s", rec.Code, rec.Body.String())
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general", "{ }", key); rec.Code != http.StatusBadRequest {
		t.Fatalf("whitespace empty object %d %s", rec.Code, rec.Body.String())
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general", "{\n}", key); rec.Code != http.StatusBadRequest {
		t.Fatalf("newline empty object %d %s", rec.Code, rec.Body.String())
	}

	if unpack.KeepHistory != 200 {
		t.Fatalf("empty put applied keep_history %d", unpack.KeepHistory)
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general",
		`{"debug":true,"notAField":1}`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field %d %s", rec.Code, rec.Body.String())
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general",
		`{"debug":true}{"debug":false}`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("extra json %d %s", rec.Code, rec.Body.String())
	}

	admin := unpack.Webserver.adminAPIKey()
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		`{"error":"forbidden"}`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("error object %d %s", rec.Code, rec.Body.String())
	}

	if unpack.Webserver.adminAPIKey() != admin {
		t.Fatal("forbidden payload wiped live admin key")
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", `[null]`, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("nil sonarr %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigPutFoldersValidationDoesNotApply(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	body := `{"interval":"1s","buffer":1000,"folder":[{"path":"/rejected/path","maxBytes":"bogus"}]}`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/folders", body, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus folder %d %s", rec.Code, rec.Body.String())
	}

	if len(unpack.Folders) != 0 {
		t.Fatalf("rejected folder went live: %+v", unpack.Folders)
	}

	if len(unpack.fileConfig.Folders) != 0 {
		t.Fatalf("rejected folder staged: %+v", unpack.fileConfig.Folders)
	}

	emptyPath := `{"interval":"1s","buffer":1000,"folder":{"foo2":{"delete_original":true}}}`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/folders", emptyPath, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty folder path %d %s", rec.Code, rec.Body.String())
	}
}

func TestConfigPutWebhooksValidationDoesNotApply(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	body := `[{"name":"broken"},{"name":"ok","url":"http://127.0.0.1:1/hook"}]`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/webhooks", body, key); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken webhook %d %s", rec.Code, rec.Body.String())
	}

	if len(unpack.Webhook) != 0 {
		t.Fatalf("rejected webhooks went live: %+v", unpack.Webhook)
	}
}

func TestConfigPutSonarrPreservesQueueAndPath(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	queued := &sonarr.Queue{}
	app := &SonarrConfig{Queue: queued}
	app.URL = "http://127.0.0.1:8989"
	app.APIKey = strings.Repeat("k", apiKeyMinLength)
	unpack.Sonarr = instanceMap([]*SonarrConfig{app})

	body := `[{"url":"http://127.0.0.1:8989","apiKey":"` + strings.Repeat("k", apiKeyMinLength) + `","path":"/dl"}]`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, key); rec.Code != http.StatusOK {
		t.Fatalf("first put %d %s", rec.Code, rec.Body.String())
	}

	if unpack.Sonarr["0"].Queue != queued {
		t.Fatal("PUT dropped last-known Starr queue")
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, key); rec.Code != http.StatusOK {
		t.Fatalf("second put %d %s", rec.Code, rec.Body.String())
	}

	var hits int

	for _, path := range unpack.Sonarr["0"].Paths {
		if path == "/dl" {
			hits++
		}
	}

	if hits != 1 {
		t.Fatalf("path merged %d times: %+v", hits, unpack.Sonarr["0"].Paths)
	}
}

func TestConfigPutWebserverKeepsListenAndEmptyKeys(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)
	admin := unpack.Webserver.adminAPIKey()
	listen := unpack.Webserver.ListenAddr

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	web.ListenAddr = "127.0.0.1:9999"
	if len(web.APIKeys) == 0 {
		t.Fatal("expected admin key on GET")
	}

	web.APIKeys[0].Key = ""

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), key)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if unpack.Webserver.ListenAddr != listen {
		t.Fatalf("live listen changed to %s", unpack.Webserver.ListenAddr)
	}

	if unpack.fileConfig.Webserver.ListenAddr != "127.0.0.1:9999" {
		t.Fatalf("file listen %s", unpack.fileConfig.Webserver.ListenAddr)
	}

	if !reply.RestartRequired {
		t.Fatal("listen change must require restart")
	}

	if unpack.Webserver.adminAPIKey() != admin {
		t.Fatal("empty api key did not keep existing key by name")
	}
}

func TestConfigPutURLBaseRoundTripNeedsNoRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.URLBase = "/unpackerr/"
	unpack.snapshotFileConfig()
	unpack.fileConfig.Webserver.URLBase = "unpackerr"
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", got.Body.String(), key)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if reply.RestartRequired {
		t.Fatal("normalized urlbase round trip must not require restart")
	}
}

// Saving a role (or any auth-only field) must not re-exec when live listen
// differs from the file because of UN_WEBSERVER_LISTEN_ADDR.
func TestConfigPutWebserverRoleWithEnvListenNeedsNoRestart(t *testing.T) { //nolint:funlen
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	unpack.Webserver.ListenAddr = "127.0.0.1:5656"
	unpack.fileConfig.Webserver.ListenAddr = "0.0.0.0:5656"
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	if web.ListenAddr != "0.0.0.0:5656" {
		t.Fatalf("GET must return file listen, got %s", web.ListenAddr)
	}

	if web.Roles == nil {
		web.Roles = map[string]Role{}
	}

	web.Roles["stats"] = Role{Permissions: []string{PermReadSystemStats}}

	if len(web.APIKeys) == 0 {
		t.Fatal("expected admin key on GET")
	}

	web.APIKeys[0].Key = ""

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), key)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if reply.RestartRequired || unpack.pendingRestart {
		t.Fatal("adding a role must not restart when only env listen differs from the file")
	}

	if unpack.Webserver.ListenAddr != "127.0.0.1:5656" {
		t.Fatalf("live listen changed to %s", unpack.Webserver.ListenAddr)
	}

	if _, ok := unpack.fileConfig.Webserver.Roles["stats"]; !ok {
		t.Fatalf("role missing from file: %+v", unpack.fileConfig.Webserver.Roles)
	}

	if _, ok := unpack.Webserver.Roles["stats"]; !ok {
		t.Fatalf("role missing from live: %+v", unpack.Webserver.Roles)
	}
}

func TestConfigPutURLBaseRejectsBraces(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	web.URLBase = "/foo/{tenant}/"

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), key)
	if put.Code != http.StatusBadRequest {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if !strings.Contains(put.Body.String(), "urlbase must not contain") {
		t.Fatalf("put body %s", put.Body.String())
	}
}

func TestConfigPutWriteFailureLeavesLiveUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(dir, "unpackerr.conf")
	unpack.KeepHistory = 200
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/general", got.Body.String(), key); rec.Code != http.StatusOK {
		t.Fatalf("seed put %d %s", rec.Code, rec.Body.String())
	}

	unpack.ConfigFile = blockedPath(t, "unpackerr.conf")

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	general.KeepHistory = 12

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	fail := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), key)
	if fail.Code != http.StatusInternalServerError {
		t.Fatalf("write fail %d %s", fail.Code, fail.Body.String())
	}

	if unpack.KeepHistory != 200 {
		t.Fatalf("live keep_history after write fail: %d", unpack.KeepHistory)
	}
}

func TestWatchWorkThreadStartsWithoutApps(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.watchWorkThread()

	done := make(chan struct{})

	go func() {
		unpack.workChan <- []func(){func() { close(done) }}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retrieveAppQueues would hang: no workChan consumer")
	}
}

func TestConfigPutWebserverDoesNotRaceAuth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	body := got.Body.String()
	admin := unpack.Webserver.adminAPIKey()

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	var wait sync.WaitGroup
	wait.Add(2)

	hit := func(method, target, payload string) {
		var reader io.Reader
		if payload != "" {
			reader = strings.NewReader(payload)
		}

		req := httptest.NewRequestWithContext(ctx, method, target, reader)
		req.Header.Set(headerAPIKey, admin)
		unpack.Webserver.router.ServeHTTP(httptest.NewRecorder(), req)
	}

	go func() {
		defer wait.Done()

		for ctx.Err() == nil {
			hit(http.MethodGet, "/api/auth/me", "")
		}
	}()

	go func() {
		defer wait.Done()

		for ctx.Err() == nil {
			hit(http.MethodPut, "/api/config/webserver", body)
		}
	}()

	wait.Wait()
	// A PUT whose request context expired may still be applying on the loop; drain before TempDir cleanup.
	_ = unpack.onMainLoop(t.Context(), func() error { return nil })
}

func TestConfigPutDebugQuietRequiresRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Config.Debug = false
	unpack.Quiet = false
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	general.Debug = true
	general.Quiet = true

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), key)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if !reply.RestartRequired {
		t.Fatal("debug/quiet must require restart")
	}

	if !unpack.Config.Debug || !unpack.Quiet {
		t.Fatal("debug/quiet must still apply live")
	}
}

func TestConfigPutWebserverKeepsFileKeysOffLiveOverlay(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	fileKey, liveKey := splitFileAndLiveAdminKeys(t, unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, liveKey)
	})
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	for idx := range web.APIKeys {
		web.APIKeys[idx].Key = ""
	}

	body, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(body), func(req *http.Request) {
		req.Header.Set(headerAPIKey, liveKey)
	})
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	assertFileAndLiveAdminKeys(t, unpack, fileKey, liveKey)
}

func splitFileAndLiveAdminKeys(t *testing.T, unpack *Unpackerr) (string, string) {
	t.Helper()

	fileKey := strings.Repeat("F", apiKeyMinLen)
	liveKey := strings.Repeat("L", apiKeyMinLen)
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   liveKey,
		Roles: []string{RoleAdmin},
	}}

	if err := unpack.Webserver.validateAuth(); err != nil {
		t.Fatal(err)
	}

	unpack.snapshotFileConfig()
	unpack.fileConfig.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   fileKey,
		Roles: []string{RoleAdmin},
	}}

	return fileKey, liveKey
}

func assertFileAndLiveAdminKeys(t *testing.T, unpack *Unpackerr, fileKey, liveKey string) {
	t.Helper()

	if unpack.Webserver.adminAPIKey() != liveKey {
		t.Fatal("live overlay key must remain the runtime secret")
	}

	if unpack.fileConfig.Webserver.APIKeys[0].Key != fileKey {
		t.Fatalf("file key %q", unpack.fileConfig.Webserver.APIKeys[0].Key)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, liveKey) {
		t.Fatalf("live overlay leaked into the config file:\n%s", text)
	}

	if !strings.Contains(text, fileKey) {
		t.Fatalf("file key missing from config file:\n%s", text)
	}
}

func TestEnsureWorkThreadsGrowsWithApps(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.watchWorkThread()

	if unpack.workThreads != 1 {
		t.Fatalf("floor workers %d", unpack.workThreads)
	}

	unpack.Sonarr = instanceMap([]*SonarrConfig{{}, {}, {}})
	unpack.ensureWorkThreads(unpack.starrAppCount())

	if unpack.workThreads != 3 {
		t.Fatalf("grown workers %d", unpack.workThreads)
	}

	unpack.ensureWorkThreads(1)

	if unpack.workThreads != 3 {
		t.Fatalf("pool shrank to %d", unpack.workThreads)
	}
}

type holdResponseWriter struct {
	http.ResponseWriter
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *holdResponseWriter) hold() {
	w.once.Do(func() { close(w.started) })
	<-w.release
}

func (w *holdResponseWriter) WriteHeader(code int) {
	w.hold()
	w.ResponseWriter.WriteHeader(code)
}

func (w *holdResponseWriter) Write(p []byte) (int, error) {
	w.hold()

	wrote, err := w.ResponseWriter.Write(p)
	if err != nil {
		return wrote, fmt.Errorf("write response: %w", err)
	}

	return wrote, nil
}

func TestConfigGetReleasesLockBeforeWrite(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	admin := unpack.Webserver.adminAPIKey()
	hold := &holdResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
		started:        make(chan struct{}),
		release:        make(chan struct{}),
	}

	var finished sync.WaitGroup

	finished.Go(func() {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/config/sonarr", nil)
		req.Header.Set(headerAPIKey, admin)
		unpack.Webserver.router.ServeHTTP(hold, req)
	})

	select {
	case <-hold.started:
	case <-time.After(2 * time.Second):
		close(hold.release)
		finished.Wait()
		t.Fatal("GET never reached response write")
	}

	putBody := `[{"url":"http://127.0.0.1:8989","apiKey":"` + strings.Repeat("k", apiKeyMinLength) + `"}]`
	putDone := make(chan int, 1)

	go func() {
		body := strings.NewReader(putBody)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/config/sonarr", body)
		req.Header.Set(headerAPIKey, admin)

		rec := httptest.NewRecorder()
		unpack.Webserver.router.ServeHTTP(rec, req)

		putDone <- rec.Code
	}()

	select {
	case code := <-putDone:
		if code != http.StatusOK {
			close(hold.release)
			finished.Wait()
			t.Fatalf("PUT during GET write: %d", code)
		}
	case <-time.After(2 * time.Second):
		close(hold.release)
		finished.Wait()
		t.Fatal("PUT blocked while GET held the response writer; configMu is still held across encode")
	}

	close(hold.release)
	finished.Wait()
}

// A filepath: Starr key must expand for the live client and stay filepath: on disk.
func TestConfigPutStarrFilepathKeyExpandsLiveOnly(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	secret := strings.Repeat("s", apiKeyMinLength)
	keyFile := filepath.Join(t.TempDir(), "sonarr.key")

	if err := os.WriteFile(keyFile, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := &SonarrConfig{}
	app.URL = "http://127.0.0.1:8989"
	app.APIKey = filePrefix + keyFile
	unpack.fileConfig.Sonarr = instanceMap([]*SonarrConfig{app})

	body, err := json.Marshal([]map[string]string{{"url": "http://127.0.0.1:8989", "apiKey": filePrefix + keyFile}})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", string(body), func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})

	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if got := unpack.Sonarr["0"].APIKey; got != secret {
		t.Fatalf("live api key %q, want the file contents", got)
	}

	if got := unpack.fileConfig.Sonarr["0"].APIKey; got != "filepath:"+keyFile {
		t.Fatalf("file api key %q, want filepath: kept", got)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	// The TOML writer escapes path separators, so match the prefix, not the full path.
	if !strings.Contains(string(written), `api_key = "filepath:`) || strings.Contains(string(written), secret) {
		t.Fatalf("config on disk must keep filepath: and never the secret:\n%s", written)
	}
}

func TestRejectAddedFilepaths(t *testing.T) {
	t.Parallel()

	existing := generalConfig{Passwords: StringSlice{filePrefix + "/secrets"}}

	if err := rejectAddedFilepaths(existing, generalConfig{
		Passwords: StringSlice{filePrefix + "/secrets", "inline"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := rejectAddedFilepaths(existing, generalConfig{Passwords: StringSlice{"literal"}}); err != nil {
		t.Fatal(err)
	}

	if err := rejectAddedFilepaths(nil, generalConfig{Passwords: StringSlice{filePrefix + "/etc/passwd"}}); err == nil ||
		!errors.Is(err, errNewFilepath) {
		t.Fatalf("new filepath: %v", err)
	}

	changed := generalConfig{Passwords: StringSlice{filePrefix + "/other"}}
	if err := rejectAddedFilepaths(existing, changed); err == nil || !errors.Is(err, errNewFilepath) {
		t.Fatalf("changed filepath: %v", err)
	}
}

func TestConfigPutRejectsNewStarrFilepath(t *testing.T) {
	t.Parallel()

	secret := strings.Repeat("s", apiKeyMinLength)
	keyFile := filepath.Join(t.TempDir(), "sonarr.key")

	if err := os.WriteFile(keyFile, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	raw, err := json.Marshal([]map[string]string{
		{"url": "http://127.0.0.1:8989", "apiKey": filePrefix + keyFile},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", string(raw), putKey(unpack))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("new filepath: put %d %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), errNewFilepath.Error()) {
		t.Fatalf("want %q, got %s", errNewFilepath, rec.Body.String())
	}

	if len(unpack.Sonarr) != 0 {
		t.Fatalf("rejected put went live: %+v", unpack.Sonarr)
	}

	if len(unpack.fileConfig.Sonarr) != 0 {
		t.Fatalf("rejected put staged file: %+v", unpack.fileConfig.Sonarr)
	}
}

func TestConfigPutRejectsNewPasswordFilepath(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get general %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	general.Passwords = StringSlice{filePrefix + "/run/secrets/rar"}

	gbody, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	grec := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(gbody), key)
	if grec.Code != http.StatusBadRequest || !strings.Contains(grec.Body.String(), errNewFilepath.Error()) {
		t.Fatalf("new password filepath: put %d %s", grec.Code, grec.Body.String())
	}
}

func TestConfigPutRejectsNewUIPasswordFilepath(t *testing.T) {
	t.Parallel()

	secretFile := filepath.Join(t.TempDir(), "ui.pass")
	if err := os.WriteFile(secretFile, []byte("correct-horse\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()
	key := putKey(unpack)

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", key)
	if got.Code != http.StatusOK {
		t.Fatalf("get webserver %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	web.UIPassword = CryptPass(filePrefix + secretFile)

	wbody, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	wrec := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", string(wbody), key)
	if wrec.Code != http.StatusBadRequest || !strings.Contains(wrec.Body.String(), errNewFilepath.Error()) {
		t.Fatalf("new ui_password filepath: put %d %s", wrec.Code, wrec.Body.String())
	}

	if unpack.fileConfig.Webserver.UIPassword.Val() == filePrefix+secretFile {
		t.Fatal("rejected ui_password filepath: must not land on the file snapshot")
	}
}

func TestConfigPutReplacesFilepathWithLiteral(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	app := &SonarrConfig{}
	app.URL = "http://127.0.0.1:8989"
	app.APIKey = filePrefix + "/run/secrets/sonarr"
	unpack.fileConfig.Sonarr = instanceMap([]*SonarrConfig{app})

	literal := strings.Repeat("k", apiKeyMinLength)
	body := `[{"url":"http://127.0.0.1:8989","apiKey":"` + literal + `"}]`
	rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, putKey(unpack))

	if rec.Code != http.StatusOK {
		t.Fatalf("replace filepath: put %d %s", rec.Code, rec.Body.String())
	}

	if got := unpack.Sonarr["0"].APIKey; got != literal {
		t.Fatalf("live api key %q", got)
	}

	if got := unpack.fileConfig.Sonarr["0"].APIKey; got != literal {
		t.Fatalf("file api key %q", got)
	}
}

// Restart-required sections flag the loop; live-only sections do not.
func TestConfigPutSetsPendingRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	sonarr := `[{"url":"http://127.0.0.1:8989","apiKey":"` + strings.Repeat("k", apiKeyMinLength) + `"}]`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", sonarr, withKey); rec.Code != http.StatusOK {
		t.Fatalf("sonarr %d %s", rec.Code, rec.Body.String())
	}

	if unpack.pendingRestart {
		t.Fatal("a Starr list applies live and must not request a restart")
	}

	folders, err := json.Marshal(map[string]any{
		"interval": "1s", "buffer": 1000, "folder": []map[string]string{{"path": t.TempDir()}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := doAuth(t, unpack, http.MethodPut, "/api/config/folders", string(folders), withKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("folders %d %s", rec.Code, rec.Body.String())
	}

	if !unpack.pendingRestart {
		t.Fatal("a folder list change needs the watcher rebuilt, so it must request a restart")
	}
}

// A general PUT that changes nothing must not schedule a re-exec, including on
// the second pass, when clampConfig has already filled the omitted defaults.
func TestConfigPutGeneralNoChangeNeedsNoRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	body := got.Body.String()

	for pass := 1; pass <= 2; pass++ {
		put := doAuth(t, unpack, http.MethodPut, "/api/config/general", body, withKey)
		if put.Code != http.StatusOK {
			t.Fatalf("pass %d put %d %s", pass, put.Code, put.Body.String())
		}

		var reply configWriteReply
		if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}

		if reply.RestartRequired || unpack.pendingRestart {
			t.Fatalf("pass %d: unchanged general PUT scheduled a restart", pass)
		}
	}

	// Dropping only the clamped mode fields must not restart either: the clamp
	// refills them with the values live already has.
	var fields map[string]any
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatal(err)
	}

	delete(fields, "fileMode")
	delete(fields, "dirMode")
	delete(fields, "logFileMode")

	trimmed, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(trimmed), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("omitted modes %d %s", put.Code, put.Body.String())
	}

	if unpack.pendingRestart {
		t.Fatal("omitting the clamped mode fields must not schedule a restart")
	}

	if unpack.FileMode == "" || unpack.DirMode == "" || unpack.LogFileMode == "" {
		t.Fatalf("clamp did not refill modes: %q %q %q", unpack.FileMode, unpack.DirMode, unpack.LogFileMode)
	}
}

// make dev sets UN_DEBUG; the general form PUTs the file document (debug false).
// That must keep the env overlay and must not re-exec.
func TestConfigPutGeneralEnvDebugNeedsNoRestart(t *testing.T) {
	t.Setenv("UN_DEBUG", "true")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	used, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(used.Used, unpack.EnvPrefix)
	if !unpack.Config.Debug {
		t.Fatal("expected UN_DEBUG overlay")
	}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	if general.Debug {
		t.Fatal("GET must return the file debug flag")
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", got.Body.String(), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if reply.RestartRequired || unpack.pendingRestart {
		t.Fatal("saving the file document must not restart when only UN_DEBUG differs")
	}

	if !unpack.Config.Debug {
		t.Fatal("live debug must stay the env overlay")
	}
}

// The general form fills omitted logFileMb/parallel/modes before PUT. That is
// the same as clampConfig, so it must not look like a logger change.
func TestConfigPutGeneralUIDefaultsNeedNoRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	got := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(got.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	if general.Parallel < 1 {
		general.Parallel = 1
	}

	if general.LogFileMb == 0 {
		general.LogFileMb = defaultLogFileMb
	}

	// GeneralForm.defaultMode padStarts to four octal digits even when GET
	// already has the clamped "644"/"755" strings from a previous snapshot.
	general.FileMode = unixModeLikeUI(general.FileMode, "0644")
	general.DirMode = unixModeLikeUI(general.DirMode, "0755")

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	var reply configWriteReply
	if err := json.Unmarshal(put.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}

	if reply.RestartRequired || unpack.pendingRestart {
		t.Fatal("UI default fills must not restart")
	}

	if unpack.FileMode != "644" || unpack.DirMode != "755" {
		t.Fatalf("live modes %q %q, want 644 755", unpack.FileMode, unpack.DirMode)
	}
}

// GeneralForm padStarts octal modes; clampConfig stores them without the leading zero.
func TestGeneralRestartRequiredIgnoresModePadding(t *testing.T) {
	t.Parallel()

	cur := New().Config
	clampConfig(cur)

	next := cloneConfig(cur)
	next.FileMode = "0" + cur.FileMode
	next.DirMode = "0" + cur.DirMode
	next.LogFileMode = "0" + cur.LogFileMode

	if generalRestartRequired(cur, next) {
		t.Fatal("0644 and 644 are the same bits and must not restart")
	}

	next.FileMode = "0640"
	if !generalRestartRequired(cur, next) {
		t.Fatal("a real mode change must still restart")
	}
}

func unixModeLikeUI(raw, fallback string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		s = fallback
	}

	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return fallback
	}

	return fmt.Sprintf("%04o", n&0o777)
}

// blockedPath returns a path whose parent is a regular file, so creating it
// fails with ENOTDIR for any user. Read-only directories do not work here:
// root ignores the permission bits, and Windows ignores them entirely.
func blockedPath(t *testing.T, name string) string {
	t.Helper()

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	return filepath.Join(blocker, name)
}

// A PUT that cannot persist must not leave a restart armed.
func TestConfigPutWriteFailureDoesNotArmRestart(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.snapshotFileConfig()
	unpack.ConfigFile = blockedPath(t, "unpackerr.conf")

	folders, err := json.Marshal(map[string]any{
		"interval": "1s", "buffer": 1000, "folder": []map[string]string{{"path": t.TempDir()}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := doAuth(t, unpack, http.MethodPut, "/api/config/folders", string(folders), func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unwritable config should 500: %d %s", rec.Code, rec.Body.String())
	}

	if unpack.pendingRestart {
		t.Fatal("a failed PUT changed nothing, so it must not schedule a restart")
	}
}

// History delete and clear must report a failed rewrite instead of a false 200:
// the rows come back on the next start.
func TestHistoryWriteFailureReportsError(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.KeepHistory = 10
	unpack.histPath = filepath.Join(t.TempDir(), historyFileName)
	unpack.upsertHistory(HistoryRecord{ID: "a", Path: "a", Status: IMPORTED, Updated: time.Now()})
	unpack.histPath = blockedPath(t, historyFileName)

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	del := doAuth(t, unpack, http.MethodPost, "/api/history/delete", `{"id":"a"}`, withKey)
	if del.Code != http.StatusInternalServerError {
		t.Fatalf("delete with an unwritable history file: %d %s", del.Code, del.Body.String())
	}

	cleared := doAuth(t, unpack, http.MethodPost, "/api/history/clear", "", withKey)
	if cleared.Code != http.StatusInternalServerError {
		t.Fatalf("clear with an unwritable history file: %d %s", cleared.Code, cleared.Body.String())
	}
}

// A PUT whose caller went away reports the main-loop timeout the way the queue
// and live-config handlers do, instead of blaming the payload with a 400.
func TestConfigPutCanceledRequestIsGatewayTimeout(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodPut,
		"/api/config/general", strings.NewReader(`{"interval":"2m"}`))
	req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())

	rec := httptest.NewRecorder()
	unpack.Webserver.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("canceled PUT should be 504: %d %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(unpack.ConfigFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled PUT must not write %s: %v", unpack.ConfigFile, err)
	}
}

const envReadarrURL = "http://readarr:8787/readarr"

func envReadarrUnpackerr(t *testing.T, secret string) *Unpackerr {
	t.Helper()

	t.Setenv("UN_READARR_0_URL", envReadarrURL)
	t.Setenv("UN_READARR_0_API_KEY", secret)

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return unpack
}

// A save of [] must not drop an env-only Starr instance from live.
func TestConfigPutReadarrEmptyKeepsEnv(t *testing.T) { //nolint:paralleltest // t.Setenv cannot run with t.Parallel.
	secret := strings.Repeat("R", apiKeyMinLength)
	unpack := envReadarrUnpackerr(t, secret)

	put := doAuth(t, unpack, http.MethodPut, "/api/config/readarr", `{}`, func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.fileConfig != nil && len(unpack.fileConfig.Readarr) != 0 {
		t.Fatalf("file readarr %+v", unpack.fileConfig.Readarr)
	}

	got := unpack.Readarr["0"]
	if len(unpack.Readarr) != 1 || got == nil || got.URL != envReadarrURL || got.APIKey != secret {
		t.Fatalf("live readarr %+v", unpack.Readarr)
	}
}

// A name-only stub is written to the file. Env still fills live URL/key.
func TestConfigPutReadarrNameKeepsEnv(t *testing.T) { //nolint:paralleltest // t.Setenv cannot run with t.Parallel.
	secret := strings.Repeat("R", apiKeyMinLength)
	unpack := envReadarrUnpackerr(t, secret)

	put := doAuth(t, unpack, http.MethodPut, "/api/config/readarr", `{"0":{"name":"books"}}`, func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	file := unpack.fileConfig.Readarr["0"]
	if file == nil || file.Name != "books" {
		t.Fatalf("name-only PUT missing from file: %+v", unpack.fileConfig.Readarr)
	}

	if file.URL != "" || file.APIKey != "" {
		t.Fatalf("file picked up env identity: %+v", file)
	}

	got := unpack.Readarr["0"]
	if got == nil || got.Name != "books" || got.URL != envReadarrURL || got.APIKey != secret {
		name, gotURL, key := "", "", ""
		if got != nil {
			name, gotURL, key = got.Name, got.URL, got.APIKey
		}

		t.Fatalf("live name=%q url=%q key=%q", name, gotURL, key)
	}
}

func TestConfigPutOtherSlugDoesNotWriteEnv(t *testing.T) { //nolint:paralleltest // t.Setenv cannot run with t.Parallel.
	secret := strings.Repeat("R", apiKeyMinLength)
	unpack := envReadarrUnpackerr(t, secret)

	otherKey := strings.Repeat("U", apiKeyMinLength)
	body := `{"uhd":{"url":"http://readarr-uhd:8787","apiKey":"` + otherKey + `"}}`

	put := doAuth(t, unpack, http.MethodPut, "/api/config/readarr", body, func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, secret) || strings.Contains(text, envReadarrURL) {
		t.Fatalf("env leaked into file:\n%s", text)
	}

	if !strings.Contains(text, "[readarr.uhd]") || !strings.Contains(text, "http://readarr-uhd:8787") {
		t.Fatalf("named slug missing from file:\n%s", text)
	}

	if unpack.fileConfig.Readarr["0"] != nil {
		t.Fatalf("env slug written to file: %+v", unpack.fileConfig.Readarr)
	}

	if unpack.Readarr["0"] == nil || unpack.Readarr["0"].APIKey != secret {
		t.Fatalf("live env slug %+v", unpack.Readarr)
	}

	if unpack.Readarr["uhd"] == nil || unpack.Readarr["uhd"].APIKey != otherKey {
		t.Fatalf("live uhd %+v", unpack.Readarr)
	}
}

func TestConfigGetLiveRedactsEnvAPIKey(t *testing.T) { //nolint:paralleltest // t.Setenv cannot run with t.Parallel.
	secret := strings.Repeat("R", apiKeyMinLength)
	unpack := envReadarrUnpackerr(t, secret)

	rec := doAuth(t, unpack, http.MethodGet, "/api/config/readarr/live", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("live get %d %s", rec.Code, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("env api key leaked on live GET: %s", rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), envReadarrURL) {
		t.Fatalf("live GET should still show env url: %s", rec.Body.String())
	}
}

func TestConfigGetLiveRedactsInstanceSecrets(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	app := &SonarrConfig{}
	app.URL = "http://127.0.0.1:8989"
	app.APIKey = strings.Repeat("S", apiKeyMinLength)
	app.HTTPPass = "basic-secret"
	app.Password = "native-secret"
	unpack.Sonarr = InstanceMap[SonarrConfig]{"uhd": app}
	unpack.Webhook = InstanceMap[WebhookConfig]{
		"discord": {URL: "http://hooks.example/discord", Token: "hook-token", Name: "discord"},
	}
	unpack.Cmdhook = InstanceMap[WebhookConfig]{
		"script": {Command: "/bin/true", Token: "cmd-token", Name: "script"},
	}
	unpack.snapshotFileConfig()

	key := putKey(unpack)
	secrets := []string{app.APIKey, app.HTTPPass, app.Password, "hook-token", "cmd-token"}

	for _, path := range []string{
		"/api/config/sonarr/live",
		"/api/config/webhooks/live",
		"/api/config/cmdhooks/live",
	} {
		rec := doAuth(t, unpack, http.MethodGet, path, "", key)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}

		body := rec.Body.String()
		for _, secret := range secrets {
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaked %q: %s", path, secret, body)
			}
		}
	}

	fileSonarr := doAuth(t, unpack, http.MethodGet, "/api/config/sonarr", "", key)
	if fileSonarr.Code != http.StatusOK {
		t.Fatalf("file sonarr %d %s", fileSonarr.Code, fileSonarr.Body.String())
	}

	if !strings.Contains(fileSonarr.Body.String(), app.APIKey) ||
		!strings.Contains(fileSonarr.Body.String(), app.HTTPPass) ||
		!strings.Contains(fileSonarr.Body.String(), app.Password) {
		t.Fatalf("file GET should still show Starr secrets: %s", fileSonarr.Body.String())
	}

	fileHook := doAuth(t, unpack, http.MethodGet, "/api/config/webhooks", "", key)
	if fileHook.Code != http.StatusOK || !strings.Contains(fileHook.Body.String(), "hook-token") {
		t.Fatalf("file webhooks %d %s", fileHook.Code, fileHook.Body.String())
	}

	fileCmd := doAuth(t, unpack, http.MethodGet, "/api/config/cmdhooks", "", key)
	if fileCmd.Code != http.StatusOK || !strings.Contains(fileCmd.Body.String(), "cmd-token") {
		t.Fatalf("file cmdhooks %d %s", fileCmd.Code, fileCmd.Body.String())
	}
}

func envWebhookUnpackerr(t *testing.T, envURL, envSecret string) *Unpackerr {
	t.Helper()

	t.Setenv("UN_WEBHOOK_0_URL", envURL)
	t.Setenv("UN_WEBHOOK_0_TOKEN", envSecret)

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return unpack
}

// A save of {} must not drop an env-only webhook from live.
func TestConfigPutWebhooksEmptyKeepsEnv(t *testing.T) { //nolint:paralleltest // t.Setenv cannot run with t.Parallel.
	const envURL = "http://hooks.example/env"

	unpack := envWebhookUnpackerr(t, envURL, "env-hook-token")

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webhooks", `{}`, putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.fileConfig != nil && len(unpack.fileConfig.Webhook) != 0 {
		t.Fatalf("file webhook %+v", unpack.fileConfig.Webhook)
	}

	got := unpack.Webhook["0"]
	if len(unpack.Webhook) != 1 || got == nil || got.URL != envURL || got.Token != "env-hook-token" {
		t.Fatalf("live webhook %+v", unpack.Webhook)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if strings.Contains(text, "env-hook-token") || strings.Contains(text, envURL) {
		t.Fatalf("env leaked into file:\n%s", text)
	}
}

// Env token without a URL must not 400 a save of a different slug.
func TestConfigPutWebhooksOtherSlugWhenEnvHasNoURL(t *testing.T) {
	t.Setenv("UN_WEBHOOK_0_TOKEN", "env-only-token")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webhooks",
		`{"discord":{"url":"http://hooks.example/file"}}`, putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.fileConfig.Webhook["0"] != nil {
		t.Fatalf("env slug written to file: %+v", unpack.fileConfig.Webhook)
	}

	if unpack.Webhook["0"] != nil {
		t.Fatalf("incomplete env leftover on live: %+v", unpack.Webhook["0"])
	}

	got := unpack.Webhook["discord"]
	if got == nil || got.URL != "http://hooks.example/file" {
		t.Fatalf("live discord %+v", got)
	}
}

func TestConfigPutRejectsBadInstanceSlug(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr",
		`{"starrs & stripes":{"url":"http://127.0.0.1:8989","apiKey":"`+strings.Repeat("k", apiKeyMinLength)+`"}}`, withKey)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad slug %d %s", rec.Code, rec.Body.String())
	}
}

type envFolderWatch struct {
	unpack      *Unpackerr
	watch       string
	fileExtract string
	envExtract  string
}

func envFolderExtractUnpackerr(t *testing.T) envFolderWatch {
	t.Helper()

	watch := t.TempDir()
	fileExtract := t.TempDir()
	envExtract := t.TempDir()

	t.Setenv("UN_FOLDER_watch_EXTRACT_PATH", envExtract)
	t.Setenv("UN_FOLDER_watch_DELETE_AFTER", "5m")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Folders = InstanceMap[FolderConfig]{
		"watch": {Path: watch, ExtractPath: fileExtract},
	}
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return envFolderWatch{unpack: unpack, watch: watch, fileExtract: fileExtract, envExtract: envExtract}
}

// Env extract_path overlays live; the PUT body is what lands in the file.
//
//nolint:paralleltest // t.Setenv cannot run with t.Parallel.
func TestConfigPutFoldersKeepsEnvExtractPath(t *testing.T) {
	setup := envFolderExtractUnpackerr(t)
	if got := setup.unpack.Folders["watch"]; got == nil || got.ExtractPath != setup.envExtract {
		t.Fatalf("startup env extract %+v", got)
	}

	body, err := json.Marshal(map[string]any{
		"interval": "2s",
		"buffer":   1000,
		"folder": map[string]any{
			"watch": map[string]any{
				"path":         setup.watch,
				"extract_path": setup.fileExtract,
				"delete_after": "10m",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, setup.unpack, http.MethodPut, "/api/config/folders", string(body), putKey(setup.unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	live := setup.unpack.Folders["watch"]
	if live == nil || live.ExtractPath != setup.envExtract {
		t.Fatalf("live extract_path after PUT %+v", live)
	}

	if live.DeleteAfter == nil || live.DeleteAfter.Duration != 5*time.Minute {
		t.Fatalf("live delete_after %+v", live.DeleteAfter)
	}

	file := setup.unpack.fileConfig.Folders["watch"]
	if file == nil || file.ExtractPath != setup.fileExtract || file.Path != setup.watch {
		t.Fatalf("file after PUT %+v", file)
	}

	if file.DeleteAfter == nil || file.DeleteAfter.Duration != 10*time.Minute {
		t.Fatalf("file delete_after %+v", file.DeleteAfter)
	}
}

type envFolderExcludes struct {
	unpack *Unpackerr
	watch  string
}

func envFolderExcludeUnpackerr(t *testing.T) envFolderExcludes {
	t.Helper()

	watch := t.TempDir()

	t.Setenv("UN_FOLDER_watch_EXCLUDE_PATH_0", "/c")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Folders = InstanceMap[FolderConfig]{
		"watch": {Path: watch, ExcludePaths: []string{"/a", "/b"}},
	}
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return envFolderExcludes{unpack: unpack, watch: watch}
}

// Indexed env must not wipe sibling exclude_paths out of the file commit.
//
//nolint:paralleltest // t.Setenv cannot run with t.Parallel.
func TestConfigPutFoldersKeepsSiblingExcludePaths(t *testing.T) {
	setup := envFolderExcludeUnpackerr(t)

	body, err := json.Marshal(map[string]any{
		"interval": "2s",
		"buffer":   1000,
		"folder": map[string]any{
			"watch": map[string]any{
				"path":          setup.watch,
				"exclude_paths": []string{"/a", "/b"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, setup.unpack, http.MethodPut, "/api/config/folders", string(body), putKey(setup.unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	live := setup.unpack.Folders["watch"]
	if live == nil || len(live.ExcludePaths) != 2 || live.ExcludePaths[0] != "/c" || live.ExcludePaths[1] != "/b" {
		t.Fatalf("live exclude_paths after PUT %+v", live)
	}

	file := setup.unpack.fileConfig.Folders["watch"]
	if file == nil || len(file.ExcludePaths) != 2 || file.ExcludePaths[0] != "/a" || file.ExcludePaths[1] != "/b" {
		t.Fatalf("file after PUT %+v", file)
	}
}

// Env DELETE_ORIGINAL without a path must not 400 a save of a different slug.
func TestConfigPutFoldersOtherSlugWhenEnvHasNoPath(t *testing.T) {
	t.Setenv("UN_FOLDER_foo2_DELETE_ORIGINAL", "true")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	watch := t.TempDir()

	body, err := json.Marshal(map[string]any{
		"interval": "1s",
		"buffer":   1000,
		"folder": map[string]any{
			"tv": map[string]any{"path": watch},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/folders", string(body), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.fileConfig.Folders["foo2"] != nil {
		t.Fatalf("env slug written to file: %+v", unpack.fileConfig.Folders)
	}

	if unpack.Folders["foo2"] != nil {
		t.Fatalf("incomplete env leftover on live: %+v", unpack.Folders["foo2"])
	}

	got := unpack.Folders["tv"]
	if got == nil || got.Path != watch {
		t.Fatalf("live tv %+v", got)
	}
}

func envSonarrURLUnpackerr(t *testing.T, secret string) *Unpackerr {
	t.Helper()

	t.Setenv("UN_SONARR_0_URL", "http://127.0.0.1:8989")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	app := &SonarrConfig{}
	app.URL = "http://file.invalid:8989"
	app.APIKey = secret
	app.Name = "uhd"
	app.Paths = StringSlice{"/downloads/tv"}
	unpack.Sonarr = InstanceMap[SonarrConfig]{"0": app}
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return unpack
}

// File API key + env URL must survive a config PUT; ParseENV overlays the URL on live.
//
//nolint:paralleltest // t.Setenv cannot run with t.Parallel.
func TestConfigPutSonarrKeepsFileAPIKeyWhenURLIsEnv(t *testing.T) {
	secret := strings.Repeat("F", apiKeyMinLength)
	unpack := envSonarrURLUnpackerr(t, secret)

	body, err := json.Marshal(map[string]any{
		"0": map[string]any{
			"url":    "http://file.invalid:8989",
			"apiKey": secret,
			"name":   "uhd",
			"paths":  []string{"/downloads/tv"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", string(body), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	live := unpack.Sonarr["0"]
	if live == nil || live.URL != "http://127.0.0.1:8989" || live.APIKey != secret || live.Name != "uhd" {
		t.Fatalf("live after PUT %+v", live)
	}

	file := unpack.fileConfig.Sonarr["0"]
	if file == nil || file.URL != "http://file.invalid:8989" || file.APIKey != secret || file.Name != "uhd" {
		t.Fatalf("file after PUT %+v", file)
	}
}

func envSonarrAPIKeyUnpackerr(t *testing.T, secret string) *Unpackerr {
	t.Helper()

	t.Setenv("UN_SONARR_0_API_KEY", secret)

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	app := &SonarrConfig{}
	app.URL = "http://file.invalid:8989"
	app.Name = "uhd"
	app.Paths = StringSlice{"/downloads/tv"}
	unpack.Sonarr = InstanceMap[SonarrConfig]{"0": app}
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	return unpack
}

// Official UI GET file (no apiKey) then Save. Overlay must fill UN_SONARR_0_API_KEY.
//
//nolint:paralleltest // t.Setenv cannot run with t.Parallel.
func TestConfigPutSonarrOmitsEnvAPIKey(t *testing.T) {
	secret := strings.Repeat("E", apiKeyMinLength)
	unpack := envSonarrAPIKeyUnpackerr(t, secret)

	for _, body := range []string{
		`{"0":{"url":"http://file.invalid:8989","name":"uhd","paths":["/downloads/tv"]}}`,
		`{"0":{"url":"http://file.invalid:8989","apiKey":"","name":"uhd","paths":["/downloads/tv"]}}`,
	} {
		put := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, putKey(unpack))
		if put.Code != http.StatusOK {
			t.Fatalf("put %s → %d %s", body, put.Code, put.Body.String())
		}

		live := unpack.Sonarr["0"]
		if live == nil || live.URL != "http://file.invalid:8989" || live.APIKey != secret || live.Name != "uhd" {
			t.Fatalf("live after PUT %s: %+v", body, live)
		}

		file := unpack.fileConfig.Sonarr["0"]
		if file == nil || file.URL != "http://file.invalid:8989" || file.APIKey != "" || file.Name != "uhd" {
			t.Fatalf("file after PUT %s: %+v", body, file)
		}
	}
}

// Env URL without a key must not 400 a save of a different slug.
func TestConfigPutSonarrOtherSlugWhenEnvURLHasNoKey(t *testing.T) {
	t.Setenv("UN_SONARR_0_URL", "http://127.0.0.1:8989")

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")

	fileKey := strings.Repeat("F", apiKeyMinLength)
	otherKey := strings.Repeat("U", apiKeyMinLength)
	zero := &SonarrConfig{}
	zero.URL = "http://file.invalid:8989"
	zero.APIKey = fileKey
	zero.Name = "hd"
	one := &SonarrConfig{}
	one.URL = "http://sonarr-uhd:8989"
	one.APIKey = otherKey
	one.Name = "uhd"
	unpack.Sonarr = InstanceMap[SonarrConfig]{"0": zero, "1": one}
	unpack.snapshotFileConfig()

	res, err := cnfg.ParseENV(unpack.Config, unpack.EnvPrefix)
	if err != nil {
		t.Fatal(err)
	}

	unpack.envUsed = envSuffixes(res.Used, unpack.EnvPrefix)

	body := `{"1":{"url":"http://sonarr-uhd:8989","apiKey":"` + otherKey + `","name":"uhd"}}`

	put := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.fileConfig.Sonarr["0"] != nil {
		t.Fatalf("omitted slug written to file: %+v", unpack.fileConfig.Sonarr)
	}

	if unpack.Sonarr["0"] != nil {
		t.Fatalf("incomplete env leftover on live: %+v", unpack.Sonarr["0"])
	}

	got := unpack.Sonarr["1"]
	if got == nil || got.URL != "http://sonarr-uhd:8989" || got.APIKey != otherKey {
		t.Fatalf("live slug 1 %+v", got)
	}
}
