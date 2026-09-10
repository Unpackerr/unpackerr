package unpackerr

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
