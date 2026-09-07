package unpackerr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestConfigPutGeneralEnablesTrayHistory(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 0
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

	body, err := json.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/general", string(body), withKey)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if unpack.KeepHistory != 50 {
		t.Fatalf("applied keep_history %d", unpack.KeepHistory)
	}

	if len(unpack.Items) != trayHistory {
		t.Fatalf("tray items after enabling history: %d", len(unpack.Items))
	}

	unpack.updateHistory("queued")

	if unpack.Items[0] != "queued" {
		t.Fatalf("updateHistory after enable: %+v", unpack.Items)
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

	if len(unpack.Sonarr) != 1 || unpack.Sonarr[0].URL != "http://127.0.0.1:8989" {
		t.Fatalf("sonarr %+v", unpack.Sonarr)
	}

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), "http://127.0.0.1:8989") {
		t.Fatalf("sonarr PUT missed fileConfig:\n%s", written)
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
	unpack.Sonarr = []*SonarrConfig{app}

	body := `[{"url":"http://127.0.0.1:8989","apiKey":"` + strings.Repeat("k", apiKeyMinLength) + `","path":"/dl"}]`
	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, key); rec.Code != http.StatusOK {
		t.Fatalf("first put %d %s", rec.Code, rec.Body.String())
	}

	if unpack.Sonarr[0].Queue != queued {
		t.Fatal("PUT dropped last-known Starr queue")
	}

	if rec := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", body, key); rec.Code != http.StatusOK {
		t.Fatalf("second put %d %s", rec.Code, rec.Body.String())
	}

	var hits int

	for _, path := range unpack.Sonarr[0].Paths {
		if path == "/dl" {
			hits++
		}
	}

	if hits != 1 {
		t.Fatalf("path merged %d times: %+v", hits, unpack.Sonarr[0].Paths)
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

func TestConfigPutWriteFailureLeavesLiveUnchanged(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows || os.Geteuid() == 0 {
		t.Skip("cannot chmod a directory unwritable")
	}

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

	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // need a read-only dir
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore after the read-only test

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

	unpack.Sonarr = []*SonarrConfig{{}, {}, {}}
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

	body, err := json.Marshal([]map[string]string{{"url": "http://127.0.0.1:8989", "apiKey": "filepath:" + keyFile}})
	if err != nil {
		t.Fatal(err)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/sonarr", string(body), func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})

	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	if got := unpack.Sonarr[0].APIKey; got != secret {
		t.Fatalf("live api key %q, want the file contents", got)
	}

	if got := unpack.fileConfig.Sonarr[0].APIKey; got != "filepath:"+keyFile {
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
