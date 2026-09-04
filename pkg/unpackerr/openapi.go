package unpackerr

import (
	_ "embed"
	"net/http"
	"path"

	"github.com/julienschmidt/httprouter"
)

//go:embed openapi.json
var openapiJSON []byte

func (u *Unpackerr) registerOpenAPIRoute() {
	u.Webserver.router.GET(path.Join(u.Webserver.URLBase, "api", "openapi.json"), u.openapiHandler)
}

func (u *Unpackerr) openapiHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(openapiJSON)
}
