package unpackerr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigGetSection(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Config.Debug = true
	unpack.KeepHistory = 200
	unpack.Sonarr = []*SonarrConfig{{}}
	unpack.Sonarr[0].Path = "/downloads"
	unpack.Sonarr[0].URL = "http://127.0.0.1:8989"
	unpack.Sonarr[0].APIKey = strings.Repeat("k", apiKeyMinLength)

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/nope", "", withKey); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth %d", rec.Code)
	}

	genRec := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	if genRec.Code != http.StatusOK {
		t.Fatalf("general %d %s", genRec.Code, genRec.Body.String())
	}

	var general generalConfig
	if err := json.Unmarshal(genRec.Body.Bytes(), &general); err != nil {
		t.Fatal(err)
	}

	if !general.Debug || general.KeepHistory != 200 {
		t.Fatalf("general %+v", general)
	}

	webRec := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", withKey)
	if webRec.Code != http.StatusOK {
		t.Fatalf("webserver %d %s", webRec.Code, webRec.Body.String())
	}

	if !strings.Contains(webRec.Body.String(), `"uiPassword":"!!cryptd!!`) {
		t.Fatalf("password not stored hash: %s", webRec.Body.String())
	}

	if !strings.Contains(webRec.Body.String(), unpack.Webserver.adminAPIKey()) {
		t.Fatalf("admin must see api keys: %s", webRec.Body.String())
	}

	starrRec := doAuth(t, unpack, http.MethodGet, "/api/config/sonarr", "", withKey)
	if starrRec.Code != http.StatusOK || !strings.Contains(starrRec.Body.String(), strings.Repeat("k", apiKeyMinLength)) {
		t.Fatalf("sonarr %d %s", starrRec.Code, starrRec.Body.String())
	}
}

func TestConfigGetPermission(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	readKey := strings.Repeat("G", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"general": {Permissions: []string{PermReadConfig(SectionGeneral)}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "cfg",
		Key:   readKey,
		Roles: []string{"general"},
	})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey); rec.Code != http.StatusOK {
		t.Fatalf("general %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", withKey); rec.Code != http.StatusForbidden {
		t.Fatalf("webserver %d", rec.Code)
	}

	liveRec := doAuth(t, unpack, http.MethodGet, "/api/config/webserver/live", "", withKey)
	if liveRec.Code != http.StatusForbidden {
		t.Fatalf("webserver live %d", liveRec.Code)
	}
}

func TestConfigGetFileVsLive(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Passwords = StringSlice{"filepath:/secrets"}
	unpack.snapshotFileConfig()
	unpack.snapshotLivePasswords()
	unpack.Passwords = StringSlice{"expanded-secret"}
	unpack.fileConfig.Webserver.UIPassword = CryptPass(filePrefix + "/ui.pass")

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	fileGen := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	liveGen := doAuth(t, unpack, http.MethodGet, "/api/config/general/live", "", withKey)

	if fileGen.Code != http.StatusOK || liveGen.Code != http.StatusOK {
		t.Fatalf("file %d live %d", fileGen.Code, liveGen.Code)
	}

	if !strings.Contains(fileGen.Body.String(), "filepath:/secrets") {
		t.Fatalf("file general %s", fileGen.Body.String())
	}

	if !strings.Contains(liveGen.Body.String(), "filepath:/secrets") ||
		strings.Contains(liveGen.Body.String(), "expanded-secret") {
		t.Fatalf("live general must not expand filepath: passwords: %s", liveGen.Body.String())
	}

	fileWeb := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", withKey)
	liveWeb := doAuth(t, unpack, http.MethodGet, "/api/config/webserver/live", "", withKey)

	if !strings.Contains(fileWeb.Body.String(), filePrefix+"/ui.pass") {
		t.Fatalf("file webserver %s", fileWeb.Body.String())
	}

	if !strings.Contains(liveWeb.Body.String(), `"uiPassword":"!!cryptd!!`) {
		t.Fatalf("live webserver %s", liveWeb.Body.String())
	}
}

func TestConfigGetLiveInlinePasswords(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Passwords = StringSlice{"inline-secret"}
	unpack.snapshotFileConfig()
	unpack.Passwords = StringSlice{"env-overlay-secret"}
	unpack.snapshotLivePasswords()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	liveGen := doAuth(t, unpack, http.MethodGet, "/api/config/general/live", "", withKey)
	if liveGen.Code != http.StatusOK {
		t.Fatalf("live %d %s", liveGen.Code, liveGen.Body.String())
	}

	if !strings.Contains(liveGen.Body.String(), "env-overlay-secret") {
		t.Fatalf("live inline/env passwords %s", liveGen.Body.String())
	}
}

func TestConfigGetLiveEnvFilepathPassword(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Passwords = StringSlice{"inline-from-file"}
	unpack.snapshotFileConfig()
	unpack.Passwords = StringSlice{"filepath:/run/secrets/pw"}
	unpack.snapshotLivePasswords()
	unpack.Passwords = StringSlice{"secretA", "secretB"}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	fileGen := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	liveGen := doAuth(t, unpack, http.MethodGet, "/api/config/general/live", "", withKey)

	if fileGen.Code != http.StatusOK || liveGen.Code != http.StatusOK {
		t.Fatalf("file %d live %d", fileGen.Code, liveGen.Code)
	}

	if !strings.Contains(fileGen.Body.String(), "inline-from-file") {
		t.Fatalf("file general %s", fileGen.Body.String())
	}

	if !strings.Contains(liveGen.Body.String(), "filepath:/run/secrets/pw") ||
		strings.Contains(liveGen.Body.String(), "secretA") ||
		strings.Contains(liveGen.Body.String(), "secretB") {
		t.Fatalf("live must keep env filepath: passwords: %s", liveGen.Body.String())
	}
}

func TestConfigGetLiveEnvOverridesFilepath(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Passwords = StringSlice{"filepath:/secrets"}
	unpack.snapshotFileConfig()
	unpack.Passwords = StringSlice{"env-inline"}
	unpack.snapshotLivePasswords()

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	fileGen := doAuth(t, unpack, http.MethodGet, "/api/config/general", "", withKey)
	liveGen := doAuth(t, unpack, http.MethodGet, "/api/config/general/live", "", withKey)

	if fileGen.Code != http.StatusOK || liveGen.Code != http.StatusOK {
		t.Fatalf("file %d live %d", fileGen.Code, liveGen.Code)
	}

	if !strings.Contains(fileGen.Body.String(), "filepath:/secrets") {
		t.Fatalf("file general %s", fileGen.Body.String())
	}

	if !strings.Contains(liveGen.Body.String(), "env-inline") ||
		strings.Contains(liveGen.Body.String(), "filepath:/secrets") {
		t.Fatalf("live must show env overlay passwords: %s", liveGen.Body.String())
	}
}

func TestConfigGetWebserverKeysNeedStar(t *testing.T) {
	t.Parallel()

	unpack, adminKey, readKey := testWebserverReadUnpackerr(t)

	withRead := func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	}

	for _, path := range []string{"/api/config/webserver", "/api/config/webserver/live"} {
		assertWebserverKeysRedacted(t, doAuth(t, unpack, http.MethodGet, path, "", withRead), adminKey, readKey)
	}

	withAdmin := func(req *http.Request) {
		req.Header.Set(headerAPIKey, adminKey)
	}

	adminFile := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", withAdmin)
	adminLive := doAuth(t, unpack, http.MethodGet, "/api/config/webserver/live", "", withAdmin)

	if !strings.Contains(adminFile.Body.String(), adminKey) {
		t.Fatalf("admin file GET %s", adminFile.Body.String())
	}

	if !strings.Contains(adminLive.Body.String(), adminKey) {
		t.Fatalf("admin live GET %s", adminLive.Body.String())
	}
}

func testWebserverReadUnpackerr(t *testing.T) (*Unpackerr, string, string) {
	t.Helper()

	unpack := testAuthUnpackerr(t)
	adminKey := unpack.Webserver.adminAPIKey()
	readKey := strings.Repeat("W", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"webread": {Permissions: []string{PermReadConfig(SectionWebserver)}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "webread",
		Key:   readKey,
		Roles: []string{"webread"},
	})
	unpack.snapshotFileConfig()

	return unpack, adminKey, readKey
}

func assertWebserverKeysRedacted(t *testing.T, rec *httptest.ResponseRecorder, adminKey, readKey string) {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("webserver %d %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if strings.Contains(body, adminKey) || strings.Contains(body, readKey) {
		t.Fatalf("leaked api key: %s", body)
	}

	var payload struct {
		APIKeys []APIKey `json:"apiKeys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	if len(payload.APIKeys) == 0 {
		t.Fatalf("missing apiKeys: %s", body)
	}

	for _, key := range payload.APIKeys {
		if key.Key != "" {
			t.Fatalf("key not redacted: %+v", key)
		}

		if key.Name == "" || len(key.Roles) == 0 {
			t.Fatalf("expected name and roles: %+v", key)
		}
	}
}
