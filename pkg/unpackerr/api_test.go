package unpackerr

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
)

func TestStatsAndSystemRequireAuth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	if rec := doAuth(t, unpack, http.MethodGet, "/api/stats", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("stats unauth %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/system", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("system unauth %d", rec.Code)
	}

	key := unpack.Webserver.adminAPIKey()
	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, key)
	}

	statsRec := doAuth(t, unpack, http.MethodGet, "/api/stats", "", withKey)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("stats %d %s", statsRec.Code, statsRec.Body.String())
	}

	var stats Stats
	if err := json.Unmarshal(statsRec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}

	sysRec := doAuth(t, unpack, http.MethodGet, "/api/system", "", withKey)
	if sysRec.Code != http.StatusOK {
		t.Fatalf("system %d %s", sysRec.Code, sysRec.Body.String())
	}

	if !strings.Contains(sysRec.Body.String(), `"auth":"password"`) {
		t.Fatalf("system body %s", sysRec.Body.String())
	}

	var info systemInfo
	if err := json.Unmarshal(sysRec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}

	if info.ListenAddr != unpack.Webserver.bindAddr() {
		t.Fatalf("listenAddr %q", info.ListenAddr)
	}
}

func TestSystemReportsPortOnlyBindAddr(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.ListenAddr = " 5656 "

	rec := doAuth(t, unpack, http.MethodGet, "/api/system", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("system %d %s", rec.Code, rec.Body.String())
	}

	var info systemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}

	if info.ListenAddr != "0.0.0.0:5656" {
		t.Fatalf("listenAddr %q", info.ListenAddr)
	}
}

func TestStatsPermissionDoesNotGrantSystem(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	statKey := strings.Repeat("S", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "home",
		Key:   statKey,
		Roles: []string{"stats"},
	})

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, statKey)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/stats", "", withKey); rec.Code != http.StatusOK {
		t.Fatalf("stats %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/system", "", withKey); rec.Code != http.StatusForbidden {
		t.Fatalf("system %d", rec.Code)
	}
}

func TestMetricsRequiresMetricsPermission(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.router.Handle("GET /metrics", unpack.requirePermHTTP(
		PermReadSystemMetrics,
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write([]byte("ok\n"))
		}),
	))

	if rec := doAuth(t, unpack, http.MethodGet, "/metrics", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics unauth %d", rec.Code)
	}

	statKey := strings.Repeat("M", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"stats": {Permissions: []string{PermReadSystemStats}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "home",
		Key:   statKey,
		Roles: []string{"stats"},
	})

	if rec := doAuth(t, unpack, http.MethodGet, "/metrics", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, statKey)
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("stats key on metrics %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/metrics", "", func(req *http.Request) {
		req.Header.Set("Authorization", authHeaderBearer+unpack.Webserver.adminAPIKey())
	}); rec.Code != http.StatusOK {
		t.Fatalf("admin bearer metrics %d %s", rec.Code, rec.Body.String())
	}
}

func TestMetricsRejectsSessionAndProxyAuth(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.Webserver.router.Handle("GET /metrics", unpack.requirePermHTTP(
		PermReadSystemMetrics,
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusOK)
		}),
	))

	payload := `{"name":"admin","kdf":"` + DeriveKDF(defaultUIUser, "correct-horse") + `"}`

	logged := doAuth(t, unpack, http.MethodPost, "/api/auth/login", payload, nil)
	if logged.Code != http.StatusOK {
		t.Fatalf("login %d %s", logged.Code, logged.Body.String())
	}

	res := logged.Result()
	_ = res.Body.Close()

	withCookie := func(req *http.Request) {
		for _, cookie := range res.Cookies() {
			req.AddCookie(cookie)
		}
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/metrics", "", withCookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("session metrics %d", rec.Code)
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/stats", "", withCookie); rec.Code != http.StatusOK {
		t.Fatalf("session stats %d", rec.Code)
	}

	unpack.Webserver.UIPassword = authNone
	unpack.Webserver.allow = MakeIPs([]string{"192.0.2.1/32"})

	if rec := doAuth(t, unpack, http.MethodGet, "/metrics", "", func(req *http.Request) {
		req.RemoteAddr = "192.0.2.1:9999"
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("proxy metrics %d", rec.Code)
	}
}

func TestConfigHelp(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/help", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("help unauth %d", rec.Code)
	}

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	helpRec := doAuth(t, unpack, http.MethodGet, "/api/config/help", "", withKey)
	if helpRec.Code != http.StatusOK {
		t.Fatalf("help %d %s", helpRec.Code, helpRec.Body.String())
	}

	var help map[string]configdef.FieldHelp
	if err := json.Unmarshal(helpRec.Body.Bytes(), &help); err != nil {
		t.Fatal(err)
	}

	if help["config.general.debug"].Short == "" {
		t.Fatalf("missing general.debug help: %+v", help["config.general.debug"])
	}

	if help["config.starr.url"].Short == "" {
		t.Fatal("missing starr.url help")
	}
}

func TestLiveExport(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.ConfigFile = "/tmp/unpackerr.conf"

	withKey := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	exportRec := doAuth(t, unpack, http.MethodGet, "/api/system/export", "", withKey)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export %d %s", exportRec.Code, exportRec.Body.String())
	}

	var out map[string]string
	if err := json.Unmarshal(exportRec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out["text"], "Live Settings") || !strings.Contains(out["text"], unpack.ConfigFile) {
		t.Fatalf("export text %q", out["text"])
	}

	if !strings.Contains(out["text"], "Version:") ||
		!strings.Contains(out["text"], runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("export missing version/os %q", out["text"])
	}
}

func TestConfigEnv(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)
	unpack.envUsed = map[string]string{ //nolint:gosec // test fixtures, not live secrets
		"DEBUG":              "true",
		"SONARR_0_API_KEY":   "secret-from-env",
		"SONARR_0_HTTP_PASS": "basic-auth-pass",
	}

	if rec := doAuth(t, unpack, http.MethodGet, "/api/config/env", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("env unauth %d", rec.Code)
	}

	withAdmin := func(req *http.Request) {
		req.Header.Set(headerAPIKey, unpack.Webserver.adminAPIKey())
	}

	adminRec := doAuth(t, unpack, http.MethodGet, "/api/config/env", "", withAdmin)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("env %d %s", adminRec.Code, adminRec.Body.String())
	}

	var admin map[string]string
	if err := json.Unmarshal(adminRec.Body.Bytes(), &admin); err != nil {
		t.Fatal(err)
	}

	if admin["DEBUG"] != "true" || admin["SONARR_0_API_KEY"] != "secret-from-env" ||
		admin["SONARR_0_HTTP_PASS"] != "basic-auth-pass" {
		t.Fatalf("admin env %v", admin)
	}

	readKey := strings.Repeat("E", apiKeyMinLen)
	unpack.Webserver.Roles = map[string]Role{
		"envread": {Permissions: []string{PermReadConfig(SectionGeneral)}},
	}
	unpack.Webserver.APIKeys = append(unpack.Webserver.APIKeys, APIKey{
		Name:  "envread",
		Key:   readKey,
		Roles: []string{"envread"},
	})

	readRec := doAuth(t, unpack, http.MethodGet, "/api/config/env", "", func(req *http.Request) {
		req.Header.Set(headerAPIKey, readKey)
	})
	if readRec.Code != http.StatusOK {
		t.Fatalf("env read %d %s", readRec.Code, readRec.Body.String())
	}

	var limited map[string]string
	if err := json.Unmarshal(readRec.Body.Bytes(), &limited); err != nil {
		t.Fatal(err)
	}

	if limited["DEBUG"] != "true" || limited["SONARR_0_API_KEY"] != "" ||
		limited["SONARR_0_HTTP_PASS"] != "" {
		t.Fatalf("limited env %v", limited)
	}
}
