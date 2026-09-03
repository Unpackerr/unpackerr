package unpackerr

import (
	"net/http"
	"path"
	"time"

	"github.com/julienschmidt/httprouter"
	"golift.io/version"
)

type systemInfo struct {
	Version    string    `json:"version"`
	Revision   string    `json:"revision"`
	Started    time.Time `json:"started"`
	Uptime     string    `json:"uptime"`
	ListenAddr string    `json:"listenAddr"`
	URLBase    string    `json:"urlbase"`
	Auth       string    `json:"auth"`
	Metrics    bool      `json:"metrics"`
}

func (u *Unpackerr) registerAPIRoutes() {
	base := path.Join(u.Webserver.URLBase, "api")
	u.Webserver.router.GET(path.Join(base, "stats"), u.requirePerm(PermReadSystemStats, u.statsHandler))
	u.Webserver.router.GET(path.Join(base, "system"), u.requirePerm(PermReadSystemInfo, u.systemHandler))
}

func (u *Unpackerr) statsHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writeJSON(response, http.StatusOK, u.stats())
}

func (u *Unpackerr) systemHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writeJSON(response, http.StatusOK, systemInfo{
		Version:    version.Version,
		Revision:   version.Revision,
		Started:    version.Started,
		Uptime:     time.Since(version.Started).Round(time.Second).String(),
		ListenAddr: u.Webserver.ListenAddr,
		URLBase:    u.Webserver.URLBase,
		Auth:       u.Webserver.UIPassword.Type().String(),
		Metrics:    u.Webserver.Metrics,
	})
}
