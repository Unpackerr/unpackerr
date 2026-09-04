package unpackerr

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/julienschmidt/httprouter"
)

func testAuthUnpackerr(t *testing.T) *Unpackerr {
	t.Helper()

	unpack := New()
	unpack.Webserver.URLBase = "/"
	unpack.Webserver.ListenAddr = "127.0.0.1:0"

	if err := unpack.Webserver.UIPassword.SetPlain(defaultUIUser, "correct-horse"); err != nil {
		t.Fatal(err)
	}

	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   strings.Repeat("A", apiKeyMinLen),
		Roles: []string{RoleAdmin},
	}}

	unpack.Webserver.router = httprouter.New()

	if err := unpack.Webserver.initCookies(); err != nil {
		t.Fatal(err)
	}

	unpack.webRoutes()

	return unpack
}

func doAuth(
	t *testing.T, unpack *Unpackerr, method, target, body string, setup func(*http.Request),
) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, target, reader)
	if setup != nil {
		setup(req)
	}

	rec := httptest.NewRecorder()
	unpack.Webserver.router.ServeHTTP(rec, req)

	return rec
}

func TestGenerateAPIKeyLength(t *testing.T) {
	t.Parallel()

	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}

	if len(key) < apiKeyMinLen || len(key) > apiKeyMaxLen {
		t.Fatalf("len %d", len(key))
	}
}

func TestRequestAPIKeyPrefersHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set(headerAPIKey, "from-header")
	req.Header.Set("Authorization", authHeaderBearer+"from-bearer")

	if got := requestAPIKey(req); got != "from-header" {
		t.Fatalf("got %q", got)
	}

	req.Header.Del(headerAPIKey)

	if got := requestAPIKey(req); got != "from-bearer" {
		t.Fatalf("bearer %q", got)
	}
}

func TestSetupAdminAPIKeyWritesTables(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.snapshotFileConfig()

	unpack.setupAdminAPIKey()

	if unpack.adminKeyNotice == "" || !unpack.Webserver.hasAdminKey() {
		t.Fatal("expected a generated admin key")
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	text := string(body)
	if !strings.Contains(text, "[[webserver.api_keys]]") {
		t.Fatalf("generated key must be a table, got:\n%s", text)
	}

	if !strings.Contains(text, unpack.Webserver.adminAPIKey()) {
		t.Fatal("config file must contain the generated key")
	}
}

func TestSetupAdminAPIKeyDoesNotPersistEnvKeys(t *testing.T) {
	t.Parallel()

	envKey := strings.Repeat("E", apiKeyMinLen)
	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.snapshotFileConfig()

	unpack.Webserver.APIKeys = []APIKey{{
		Name:  "from-env",
		Key:   envKey,
		Roles: []string{"stats"},
	}}

	unpack.setupAdminAPIKey()

	if !unpack.Webserver.hasAdminKey() {
		t.Fatal("expected a generated admin key")
	}

	for _, key := range unpack.fileConfig.Webserver.APIKeys {
		if key.Key == envKey {
			t.Fatal("env API key leaked into the file snapshot")
		}
	}

	body, err := os.ReadFile(unpack.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(body), envKey) {
		t.Fatalf("env API key leaked into the config file:\n%s", body)
	}
}

func TestSetupAdminAPIKeyAdoptsFileKey(t *testing.T) {
	t.Parallel()

	fileKey := strings.Repeat("F", apiKeyMinLen)
	envKey := strings.Repeat("E", apiKeyMinLen)
	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   fileKey,
		Roles: []string{RoleAdmin},
	}}
	unpack.snapshotFileConfig()
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  "from-env",
		Key:   envKey,
		Roles: []string{"stats"},
	}}

	unpack.setupAdminAPIKey()

	if unpack.Webserver.adminAPIKey() != fileKey {
		t.Fatalf("live admin %q", unpack.Webserver.adminAPIKey())
	}

	if unpack.adminKeyNotice != "" {
		t.Fatal("must not generate a second on-disk admin key")
	}

	count := 0

	for _, key := range unpack.fileConfig.Webserver.APIKeys {
		if key.Name == defaultAdminKeyName {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("file admin names %d", count)
	}
}

func TestSetupAdminAPIKeySkipsDemotedFileSecret(t *testing.T) {
	t.Parallel()

	fileKey := strings.Repeat("F", apiKeyMinLen)
	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   fileKey,
		Roles: []string{RoleAdmin},
	}}
	unpack.snapshotFileConfig()
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   fileKey,
		Roles: []string{"stats"},
	}}

	if err := unpack.Webserver.validateAuth(); err != nil {
		t.Fatal(err)
	}

	unpack.setupAdminAPIKey()

	if unpack.Webserver.adminAPIKey() == fileKey {
		t.Fatal("must not re-admin the env-demoted secret")
	}

	if !unpack.Webserver.hasAdminKey() {
		t.Fatal("expected a newly generated admin key")
	}

	if unpack.Webserver.HasPermission(fileKey, PermAll) {
		t.Fatal("demoted secret must not keep *")
	}

	if !unpack.Webserver.HasPermission(fileKey, PermReadSystemStats) {
		t.Fatal("demoted secret must keep its env roles")
	}

	if err := unpack.Webserver.validateAuth(); err != nil {
		t.Fatal(err)
	}
}

func TestSetupAdminAPIKeyAvoidsFileNames(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.ConfigFile = filepath.Join(t.TempDir(), "unpackerr.conf")
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.Webserver.APIKeys = []APIKey{{
		Name:  defaultAdminKeyName,
		Key:   strings.Repeat("a", apiKeyMinLen),
		Roles: []string{"stats"},
	}}
	unpack.snapshotFileConfig()
	unpack.Webserver.APIKeys = nil

	unpack.setupAdminAPIKey()

	if !unpack.Webserver.hasAdminKey() {
		t.Fatal("expected a generated admin key")
	}

	if unpack.Webserver.APIKeys[len(unpack.Webserver.APIKeys)-1].Name == defaultAdminKeyName {
		t.Fatal("must not reuse the file's non-admin name")
	}
}

func TestSetupAdminAPIKeySkipsDisabled(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.ListenAddr = ""

	unpack.setupAdminAPIKey()

	if unpack.Webserver.hasAdminKey() {
		t.Fatal("disabled server must not generate a key")
	}
}

func TestSetupAdminAPIKeyUnwritableIsNotFatal(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windows {
		t.Skip("directory permissions are not unix-like")
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	conf := dir + "/unpackerr.conf"

	if err := os.WriteFile(conf, []byte("debug = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o555); err != nil { //nolint:gosec // need a read-only dir
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) //nolint:gosec // restore after the read-only test

	unpack := New()
	unpack.ConfigFile = conf
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.snapshotFileConfig()

	unpack.setupAdminAPIKey()

	if unpack.configWriteErr == nil {
		t.Fatal("expected persist error")
	}

	if !unpack.Webserver.hasAdminKey() {
		t.Fatal("expected an in-memory admin key")
	}
}

func TestLoginMeLogout(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	logged := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)

	if logged.Code != http.StatusOK {
		t.Fatalf("login %d %s", logged.Code, logged.Body.String())
	}

	var login authInfo
	if err := json.Unmarshal(logged.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}

	if login.APIKey != unpack.Webserver.adminAPIKey() || login.Username != defaultUIUser {
		t.Fatalf("login payload %+v", login)
	}

	res := logged.Result()
	_ = res.Body.Close()

	meRec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		for _, cookie := range res.Cookies() {
			req.AddCookie(cookie)
		}
	})
	if meRec.Code != http.StatusOK {
		t.Fatalf("me cookie %d %s", meRec.Code, meRec.Body.String())
	}

	key := unpack.Webserver.adminAPIKey()
	headerRec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, key)
	})

	if headerRec.Code != http.StatusOK {
		t.Fatalf("me key %d %s", headerRec.Code, headerRec.Body.String())
	}

	bearerRec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		req.Header.Set("Authorization", authHeaderBearer+key)
	})

	if bearerRec.Code != http.StatusOK {
		t.Fatalf("me bearer %d %s", bearerRec.Code, bearerRec.Body.String())
	}

	if out := doAuth(t, unpack, http.MethodPost, "/api/auth/logout", "", nil); out.Code != http.StatusOK {
		t.Fatalf("logout %d", out.Code)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", `{"name":"admin","kdf":"nope"}`, nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoginDisabledForWebauth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.UIPassword = "webauth:X-User"
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", `{"name":"admin","kdf":"x"}`, nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestMeUnauthorizedWithoutCreds(t *testing.T) {
	t.Parallel()

	rec := doAuth(t, testAuthUnpackerr(t), http.MethodGet, "/api/auth/me", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestNoauthMeFromUpstream(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.UIPassword = authNone
	unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})
	rec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		req.RemoteAddr = "192.0.2.1:9999"
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestUnrecognizedBearerFallsThroughToProxy(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.UIPassword = authNone
	unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})
	rec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		req.RemoteAddr = "192.0.2.1:9999"
		req.Header.Set("Authorization", authHeaderBearer+"not-an-unpackerr-key")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestUnrecognizedBearerFallsThroughToSession(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	logged := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)

	if logged.Code != http.StatusOK {
		t.Fatalf("login %d %s", logged.Code, logged.Body.String())
	}

	res := logged.Result()
	_ = res.Body.Close()

	rec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		for _, cookie := range res.Cookies() {
			req.AddCookie(cookie)
		}

		req.Header.Set("Authorization", authHeaderBearer+"proxy-oauth-token")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}

	badKey := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		for _, cookie := range res.Cookies() {
			req.AddCookie(cookie)
		}

		req.Header.Set(headerAPIKey, strings.Repeat("x", apiKeyMinLen))
	})
	if badKey.Code != http.StatusUnauthorized {
		t.Fatalf("explicit bad api key %d", badKey.Code)
	}
}

func TestAuthJSONNotCached(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	logged := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)

	if logged.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login cache %q", logged.Header().Get("Cache-Control"))
	}

	res := logged.Result()
	_ = res.Body.Close()

	meRec := doAuth(t, unpack, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		for _, cookie := range res.Cookies() {
			req.AddCookie(cookie)
		}
	})
	if meRec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("me cache %q", meRec.Header().Get("Cache-Control"))
	}
}

func TestSessionCookieNotSecureOnHTTP(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)
	res := rec.Result()
	_ = res.Body.Close()

	cookies := res.Cookies()
	if rec.Code != http.StatusOK || len(cookies) != 1 || cookies[0].Secure {
		t.Fatalf("plaintext login %d cookies=%v", rec.Code, cookies)
	}
}

func TestLoginSessionEncodeError(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.cookies = securecookie.New(nil, nil)
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginUnregisteredWithoutCookies(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.URLBase = "/"
	unpack.Webserver.ListenAddr = "127.0.0.1:0"
	unpack.Webserver.router = httprouter.New()
	unpack.webRoutes()

	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", `{}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestSessionCookieSecureFromForwardedProto(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, func(req *http.Request) {
		req.Header.Set(headerForwardedProto, "https")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}

	res := rec.Result()
	_ = res.Body.Close()

	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("trusted https proto should set Secure, cookies=%v", cookies)
	}
}

func TestSessionCookieUsesNearestForwardedProto(t *testing.T) {
	t.Parallel()

	tests := []struct {
		proto  string
		secure bool
	}{
		{proto: "http, https", secure: true},
		{proto: "https, http", secure: false},
	}

	for _, test := range tests {
		t.Run(test.proto, func(t *testing.T) {
			t.Parallel()

			unpack := testAuthUnpackerr(t)
			unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})
			payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
			rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, func(req *http.Request) {
				req.Header.Set(headerForwardedProto, test.proto)
			})

			res := rec.Result()
			_ = res.Body.Close()

			cookies := res.Cookies()
			if rec.Code != http.StatusOK || len(cookies) != 1 || cookies[0].Secure != test.secure {
				t.Fatalf("proto %q code %d cookies=%v", test.proto, rec.Code, cookies)
			}
		})
	}
}

func TestSessionCookieIgnoresUntrustedForwardedProto(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, func(req *http.Request) {
		req.RemoteAddr = "198.51.100.1:9"
		req.Header.Set(headerForwardedProto, "https")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}

	res := rec.Result()
	_ = res.Body.Close()

	cookies := res.Cookies()
	if len(cookies) != 1 || cookies[0].Secure {
		t.Fatalf("untrusted proto must not set Secure, cookies=%v", cookies)
	}
}

func TestSessionCookieSecureFromTLSFiles(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.SSLCrtFile = "cert.pem"
	unpack.Webserver.SSLKeyFile = "key.pem"
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	rec := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}

	res := rec.Result()
	_ = res.Body.Close()

	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure {
		t.Fatalf("local TLS should set Secure, cookies=%v", cookies)
	}
}

type deadlineRecorder struct {
	http.ResponseWriter
	deadline time.Time
}

func (rec *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	rec.deadline = deadline
	return nil
}

type opaqueWriter struct {
	http.ResponseWriter
}

func TestLoginReadDeadlineAppliesOutsideLogger(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	raw := &deadlineRecorder{ResponseWriter: httptest.NewRecorder()}

	var (
		seen     time.Time
		innerErr error
	)

	handler := unpack.withLoginReadDeadline(http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			seen = raw.deadline
			innerErr = http.NewResponseController(opaqueWriter{ResponseWriter: response}).
				SetReadDeadline(time.Time{})
		}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, unpack.loginPath(), nil)
	handler.ServeHTTP(raw, req)

	if seen.IsZero() {
		t.Fatal("login deadline must be set on the raw writer before apachelog wraps it")
	}

	if !errors.Is(innerErr, http.ErrNotSupported) {
		t.Fatalf("logged writer should hide SetReadDeadline, got %v", innerErr)
	}

	raw = &deadlineRecorder{ResponseWriter: httptest.NewRecorder()}
	seen = time.Time{}
	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/auth/me", nil)
	handler.ServeHTTP(raw, req)

	if !seen.IsZero() {
		t.Fatal("non-login requests must not get a login read deadline")
	}
}

func TestLoginSucceedsThroughReadDeadlineMiddleware(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.failDelay = 0
	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, unpack.loginPath(), strings.NewReader(payload))
	rec := httptest.NewRecorder()
	unpack.withLoginReadDeadline(unpack.Webserver.router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
}
