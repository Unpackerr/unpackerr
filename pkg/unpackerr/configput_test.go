package unpackerr

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPutGeneralRoundTrip(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.KeepHistory = 200
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

	written, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(written)
	if !strings.Contains(text, "keep_history = 50") || !strings.Contains(text, "debug = true") {
		t.Fatalf("fileConfig write missed PUT:\n%s", text)
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
