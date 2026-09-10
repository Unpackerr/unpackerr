package unpackerr

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func getWebserverPUT(t *testing.T, unpack *Unpackerr) WebServer {
	t.Helper()

	got := doAuth(t, unpack, http.MethodGet, "/api/config/webserver", "", putKey(unpack))
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}

	var web WebServer
	if err := json.Unmarshal(got.Body.Bytes(), &web); err != nil {
		t.Fatal(err)
	}

	return web
}

func TestConfigPutWebserverRejectsPlaintextPassword(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = "admin:new-secret-99"

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "correct-horse")), putKey(unpack))
	if put.Code != http.StatusBadRequest || !strings.Contains(put.Body.String(), "kdf-hex") {
		t.Fatalf("plaintext put %d %s", put.Code, put.Body.String())
	}
}

func TestConfigPutWebserverAcceptsKDFPassword(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = CryptPass("admin:" + DeriveKDF("admin", "new-secret-99"))

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "correct-horse")), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("kdf put %d %s", put.Code, put.Body.String())
	}

	if !unpack.Webserver.UIPassword.ValidPlain("admin", "new-secret-99") {
		t.Fatal("live password must accept the new secret")
	}

	if unpack.fileConfig.Webserver.UIPassword.Val() != "" &&
		!unpack.fileConfig.Webserver.UIPassword.IsCrypted() {
		t.Fatalf("file must store hash, got %q", unpack.fileConfig.Webserver.UIPassword)
	}
}

func TestConfigPutWebserverKDFNeedsCurrentPassword(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = CryptPass("admin:" + DeriveKDF("admin", "new-secret-99"))

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", mustJSON(t, web), putKey(unpack))
	if put.Code != http.StatusBadRequest || !strings.Contains(put.Body.String(), "current ui password") {
		t.Fatalf("missing current %d %s", put.Code, put.Body.String())
	}

	put = doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "wrong-horse1")), putKey(unpack))
	if put.Code != http.StatusBadRequest {
		t.Fatalf("wrong current %d %s", put.Code, put.Body.String())
	}
}

func TestConfigPutWebserverRoundTripHashNeedsNoCurrent(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", mustJSON(t, web), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("round-trip %d %s", put.Code, put.Body.String())
	}
}

func TestConfigPutWebserverSwitchToHeader(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = "webauth:X-Remote-User"
	web.Upstreams = StringSlice{"192.0.2.1/32"}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "correct-horse")), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("header put %d %s", put.Code, put.Body.String())
	}

	if unpack.Webserver.UIPassword.Type() != AuthHeader ||
		unpack.Webserver.UIPassword.Header() != "X-Remote-User" {
		t.Fatalf("live auth %s %s", unpack.Webserver.UIPassword.Type(), unpack.Webserver.UIPassword.Header())
	}

	web = getWebserverPUT(t, unpack)
	web.UIPassword = CryptPass("dave:" + DeriveKDF("dave", "header-to-pass"))

	put = doAuth(t, unpack, http.MethodPut, "/api/config/webserver", mustJSON(t, web), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("header to password %d %s", put.Code, put.Body.String())
	}

	if !unpack.Webserver.UIPassword.ValidPlain("dave", "header-to-pass") {
		t.Fatal("switching from header must not require a current password")
	}
}

func TestConfigPutWebserverSwitchToNoauth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = authNone

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "correct-horse")), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("noauth put %d %s", put.Code, put.Body.String())
	}

	if unpack.Webserver.UIPassword.Type() != AuthNone {
		t.Fatalf("live auth %s", unpack.Webserver.UIPassword.Type())
	}
}

func TestConfigPutWebserverEnvPasswordRoundTripKeepsLive(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	var filePass, livePass CryptPass
	if err := filePass.SetPlain("fileuser", "filepass99"); err != nil {
		t.Fatal(err)
	}

	if err := livePass.SetPlain("envuser", "envsecret1"); err != nil {
		t.Fatal(err)
	}

	unpack.fileConfig.Webserver.UIPassword = filePass
	unpack.Webserver.UIPassword = livePass

	web := getWebserverPUT(t, unpack)
	if web.UIPassword.Val() != filePass.Val() {
		t.Fatalf("GET must return file password, got %q", web.UIPassword)
	}

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", mustJSON(t, web), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("round-trip overlay %d %s", put.Code, put.Body.String())
	}

	if !unpack.Webserver.UIPassword.ValidPlain("envuser", "envsecret1") {
		t.Fatal("live overlay password must remain")
	}

	if unpack.fileConfig.Webserver.UIPassword.Val() != filePass.Val() {
		t.Fatalf("file password %q", unpack.fileConfig.Webserver.UIPassword)
	}
}

func TestConfigPutWebserverOmitsPasswordKeepsLiveAndFile(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	live := unpack.Webserver.UIPassword
	file := unpack.fileConfig.Webserver.UIPassword
	web := getWebserverPUT(t, unpack)
	web.UIPassword = ""

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver", mustJSON(t, web), putKey(unpack))
	if put.Code != http.StatusOK {
		t.Fatalf("omit password %d %s", put.Code, put.Body.String())
	}

	if unpack.Webserver.UIPassword != live {
		t.Fatal("live password changed")
	}

	if unpack.fileConfig.Webserver.UIPassword != file {
		t.Fatal("file password changed")
	}
}

func TestConfigPutWebserverRejectsEmptyAuthHeader(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.snapshotFileConfig()

	web := getWebserverPUT(t, unpack)
	web.UIPassword = "webauth:"

	put := doAuth(t, unpack, http.MethodPut, "/api/config/webserver",
		mustWebPUT(t, web, DeriveKDF(defaultUIUser, "correct-horse")), putKey(unpack))
	if put.Code != http.StatusBadRequest {
		t.Fatalf("empty header %d %s", put.Code, put.Body.String())
	}
}

func TestAuthMeIncludesUpstreamHints(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", putKey(unpack))
	if rec.Code != http.StatusOK {
		t.Fatalf("me %d %s", rec.Code, rec.Body.String())
	}

	var info authInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}

	if info.Header != defaultAuthHeader {
		t.Fatalf("header %q", info.Header)
	}

	if info.ClientIP == "" {
		t.Fatal("expected clientIP")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

func mustWebPUT(t *testing.T, web WebServer, currentKDF string) string {
	t.Helper()

	if currentKDF == "" {
		return mustJSON(t, web)
	}

	raw, err := json.Marshal(web)
	if err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}

	obj["uiCurrentKdf"] = currentKDF

	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}
