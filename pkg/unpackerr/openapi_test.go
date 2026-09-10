package unpackerr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIUnauthenticated(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodGet, "/api/openapi.json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi %d %s", rec.Code, rec.Body.String())
	}

	doc := decodeOpenAPI(t, rec.Body.Bytes())
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("openapi field %v", doc["openapi"])
	}

	if openAPIDocServerURL(t, doc) != "/" {
		t.Fatalf("root servers url %v", openAPIDocServerURL(t, doc))
	}

	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/api/stats"]; !ok {
		t.Fatal("missing /api/stats")
	}

	if _, ok := paths["/api/config/{section}/live"]; !ok {
		t.Fatal("missing live config GET")
	}

	if _, ok := paths["/api/config/env"]; !ok {
		t.Fatal("missing /api/config/env")
	}

	login := openAPIPath(t, paths, "/api/auth/login")
	post, _ := login["post"].(map[string]any)
	resps, _ := post["responses"].(map[string]any)

	for _, code := range []string{"400", "401", "403"} {
		if _, ok := resps[code]; !ok {
			t.Fatalf("login missing %s", code)
		}
	}

	metrics := openAPIPath(t, paths, "/metrics")
	get, _ := metrics["get"].(map[string]any)

	sec, _ := get["security"].([]any)
	if len(sec) != 2 {
		t.Fatalf("metrics security %v", sec)
	}

	retry := openAPIPath(t, paths, "/api/queue/retry")
	retryPost, _ := retry["post"].(map[string]any)
	retryResps, _ := retryPost["responses"].(map[string]any)

	for _, code := range []string{"400", "504"} {
		if _, ok := retryResps[code]; !ok {
			t.Fatalf("retry missing %s", code)
		}
	}
}

func TestOpenAPIHonorsURLBase(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.Webserver.URLBase = "/unpackerr/"
	unpack.Webserver.router = http.NewServeMux()
	unpack.webRoutes()

	root := httptest.NewRecorder()
	unpack.Webserver.router.ServeHTTP(root,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/openapi.json", nil))

	if root.Code != http.StatusNotFound {
		t.Fatalf("root openapi %d", root.Code)
	}

	rec := httptest.NewRecorder()
	unpack.Webserver.router.ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/unpackerr/api/openapi.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("urlbase openapi %d %s", rec.Code, rec.Body.String())
	}

	doc := decodeOpenAPI(t, rec.Body.Bytes())
	if got := openAPIDocServerURL(t, doc); got != "/unpackerr" {
		t.Fatalf("servers url %q", got)
	}
}

func TestOpenAPIServerURL(t *testing.T) {
	t.Parallel()

	if got := openAPIServerURL(""); got != "/" {
		t.Fatalf("empty %q", got)
	}

	if got := openAPIServerURL("/"); got != "/" {
		t.Fatalf("root %q", got)
	}

	if got := openAPIServerURL("/unpackerr/"); got != "/unpackerr" {
		t.Fatalf("urlbase %q", got)
	}
}

func TestOpenAPILoginRequestOnlyRequiresKDF(t *testing.T) {
	t.Parallel()

	var doc map[string]any
	if err := json.Unmarshal(openapiJSON, &doc); err != nil {
		t.Fatal(err)
	}

	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	login, _ := schemas["LoginRequest"].(map[string]any)
	required, _ := login["required"].([]any)

	if len(required) != 1 || required[0] != "kdf" {
		t.Fatalf("LoginRequest.required %v", required)
	}
}

func decodeOpenAPI(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}

	return doc
}

func openAPIDocServerURL(t *testing.T, doc map[string]any) string {
	t.Helper()

	servers, _ := doc["servers"].([]any)
	if len(servers) == 0 {
		t.Fatal("missing servers")
	}

	server, _ := servers[0].(map[string]any)
	url, _ := server["url"].(string)

	return url
}

func openAPIPath(t *testing.T, paths map[string]any, name string) map[string]any {
	t.Helper()

	item, ok := paths[name].(map[string]any)
	if !ok {
		t.Fatalf("missing %s", name)
	}

	return item
}
