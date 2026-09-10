package unpackerr

import (
	"net/http"
	"path"
	"time"

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
	basePath := func(b string) string { return path.Join(base, b) }

	u.Webserver.handleGet(basePath("stats"), u.requirePerm(PermReadSystemStats, u.statsHandler))
	u.Webserver.handleGet(basePath("system"), u.requirePerm(PermReadSystemInfo, u.systemHandler))
	u.Webserver.handleGet(basePath("queue"), u.requirePerm(PermReadSystemQueue, u.queueHandler))
	u.Webserver.handlePost(basePath("queue/retry"), u.requirePerm(PermWriteSystemQueue, u.queueRetryHandler))
	u.Webserver.handlePost(basePath("queue/forget"), u.requirePerm(PermWriteSystemQueue, u.queueForgetHandler))
	u.Webserver.handleGet(basePath("history"), u.requirePerm(PermReadSystemHistory, u.historyHandler))
	u.Webserver.handlePost(basePath("history/clear"), u.requirePerm(PermWriteSystemHistory, u.historyClearHandler))
	u.Webserver.handlePost(basePath("history/delete"), u.requirePerm(PermWriteSystemHistory, u.historyDeleteHandler))
	u.Webserver.handleGet(basePath("browse"), u.requirePerm(PermReadSystemBrowse, u.browseHandler))
	u.Webserver.handlePost(basePath("browse"), u.requirePerm(PermWriteSystemBrowse, u.browseCreateHandler))
	u.Webserver.handleGet(basePath("config/env"), u.requireAuth(u.configEnvHandler))
	u.Webserver.handleGet(basePath("config/{section}/live"), u.requireConfigPerm(false, u.configGetLiveHandler))
	u.Webserver.handleGet(basePath("config/{section}"), u.requireConfigPerm(false, u.configGetHandler))
	u.Webserver.handlePut(basePath("config/{section}"), u.requireConfigPerm(true, u.configPutHandler))
}

func (u *Unpackerr) statsHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.stats())
}

func (u *Unpackerr) queueHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.queueSnapshot())
}

func (u *Unpackerr) historyHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.historySnapshot())
}

func (u *Unpackerr) systemHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, systemInfo{
		Version:    version.Version,
		Revision:   version.Revision,
		Started:    version.Started,
		Uptime:     time.Since(version.Started).Round(time.Second).String(),
		ListenAddr: u.Webserver.bindAddr(),
		URLBase:    u.Webserver.URLBase,
		Auth:       u.uiPassword().Type().String(),
		Metrics:    u.Webserver.Metrics,
	})
}

func (u *Unpackerr) configEnvHandler(response http.ResponseWriter, request *http.Request) {
	info, _ := request.Context().Value(authCtxKey).(authInfo)
	writeJSON(response, http.StatusOK, u.envPairsPublic(info))
}

func (u *Unpackerr) envPairsPublic(info authInfo) map[string]string {
	used := u.envUsed
	if used == nil {
		return map[string]string{}
	}

	out := make(map[string]string, len(used))
	star := info.allows(PermAll)

	for key, val := range used {
		if !star && envValueSecret(key) {
			out[key] = ""
			continue
		}

		out[key] = val
	}

	return out
}
