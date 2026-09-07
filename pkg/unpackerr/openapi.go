package unpackerr

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"path"
	"strings"
)

//go:embed openapi.json
var openapiJSON []byte

func openAPIServerURL(urlbase string) string {
	base := strings.TrimSuffix(urlbase, "/")
	if base == "" {
		return "/"
	}

	return base
}

func openAPISpec(urlbase string) []byte {
	var doc map[string]any
	if err := json.Unmarshal(openapiJSON, &doc); err != nil {
		return openapiJSON
	}

	servers, _ := doc["servers"].([]any)
	if len(servers) == 0 {
		return openapiJSON
	}

	server, ok := servers[0].(map[string]any)
	if !ok {
		return openapiJSON
	}

	server["url"] = openAPIServerURL(urlbase)

	out, err := json.Marshal(doc)
	if err != nil {
		return openapiJSON
	}

	return out
}

func (u *Unpackerr) registerOpenAPIRoute() {
	spec := openAPISpec(u.Webserver.URLBase)
	u.Webserver.handleGet(path.Join(u.Webserver.URLBase, "api", "openapi.json"), openapiHandler(spec))
}

func openapiHandler(spec []byte) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(spec)
	}
}
