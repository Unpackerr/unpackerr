package unpackerr

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestOpenAPIUnauthenticated(t *testing.T) {
	t.Parallel()

	unpack := testAuthUnpackerr(t)

	rec := doAuth(t, unpack, http.MethodGet, "/api/openapi.json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi %d %s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}

	if doc["openapi"] != "3.0.3" {
		t.Fatalf("openapi field %v", doc["openapi"])
	}

	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/api/stats"]; !ok {
		t.Fatal("missing /api/stats")
	}

	if _, ok := paths["/api/config/{section}/live"]; !ok {
		t.Fatal("missing live config GET")
	}
}
