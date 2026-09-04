package unpackerr

import (
	"encoding/json"
	"net/http"
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

	if !strings.Contains(liveGen.Body.String(), "expanded-secret") ||
		strings.Contains(liveGen.Body.String(), "filepath:/secrets") {
		t.Fatalf("live general %s", liveGen.Body.String())
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
