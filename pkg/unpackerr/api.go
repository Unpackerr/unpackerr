package unpackerr

import (
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
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
	ConfigFile string    `json:"configFile"`
	Hostname   string    `json:"hostname"`
	GOOS       string    `json:"goos"`
	Logs       string    `json:"logs"`
}

func (u *Unpackerr) registerAPIRoutes() {
	base := path.Join(u.Webserver.URLBase, "api")
	basePath := func(b string) string { return path.Join(base, b) }

	u.Webserver.handleGet(basePath("stats"), u.requirePerm(PermReadSystemStats, u.statsHandler))
	u.Webserver.handleGet(basePath("system"), u.requirePerm(PermReadSystemInfo, u.systemHandler))
	u.Webserver.handleGet(basePath("system/export"), u.requirePerm(PermReadSystemInfo, u.systemExportHandler))
	u.Webserver.handleGet(basePath("queue"), u.requirePerm(PermReadSystemQueue, u.queueHandler))
	u.Webserver.handleGet(basePath("queue/item"), u.requirePerm(PermReadSystemQueue, u.queueItemHandler))
	u.Webserver.handlePost(basePath("queue/retry"), u.requirePerm(PermWriteSystemQueue, u.queueRetryHandler))
	u.Webserver.handlePost(basePath("queue/forget"), u.requirePerm(PermWriteSystemQueue, u.queueForgetHandler))
	u.Webserver.handleGet(basePath("history"), u.requirePerm(PermReadSystemHistory, u.historyHandler))
	u.Webserver.handlePost(basePath("history/clear"), u.requirePerm(PermWriteSystemHistory, u.historyClearHandler))
	u.Webserver.handlePost(basePath("history/delete"), u.requirePerm(PermWriteSystemHistory, u.historyDeleteHandler))
	u.Webserver.handleGet(basePath("browse"), u.requirePerm(PermReadSystemBrowse, u.browseHandler))
	u.Webserver.handlePost(basePath("browse"), u.requirePerm(PermWriteSystemBrowse, u.browseCreateHandler))
	u.Webserver.handleGet(basePath("config/help"), u.requireAuth(u.configHelpHandler))
	u.Webserver.handleGet(basePath("config/env"), u.requireAuth(u.configEnvHandler))
	u.Webserver.handleGet(basePath("config/{section}/live"), u.requireConfigPerm(false, u.configGetLiveHandler))
	u.Webserver.handleGet(basePath("config/{section}"), u.requireConfigPerm(false, u.configGetHandler))
	u.Webserver.handlePut(basePath("config/{section}"), u.requireConfigPerm(true, u.configPutHandler))
	u.Webserver.handlePost(basePath("config/{section}/test"), u.requireConfigPerm(true, u.configTestHandler))
}

func (u *Unpackerr) statsHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.stats())
}

func (u *Unpackerr) queueHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.queueSnapshot())
}

func (u *Unpackerr) queueItemHandler(response http.ResponseWriter, request *http.Request) {
	itemID := request.URL.Query().Get("id")
	if strings.TrimSpace(itemID) == "" {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errMissingID.Error()})
		return
	}

	u.rLockHistory()

	item := u.Map[itemID]
	if item == nil {
		u.rUnlockHistory()
		writeJSON(response, http.StatusNotFound, map[string]string{"error": errQueueNotFound.Error()})

		return
	}

	queue := u.queueFromExtract(itemID, item)
	fillQueueFiles(&queue, item)
	u.rUnlockHistory()

	writeJSON(response, http.StatusOK, queue)
}

func (u *Unpackerr) historyHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, u.historySnapshot())
}

func (u *Unpackerr) systemHandler(response http.ResponseWriter, request *http.Request) {
	host, _ := os.Hostname()
	info := systemInfo{
		Version:  version.Version,
		Revision: version.Revision,
		Started:  version.Started,
		Uptime:   time.Since(version.Started).Round(time.Second).String(),
		Hostname: host,
		GOOS:     runtime.GOOS,
	}

	err := u.onMainLoop(request.Context(), func() error {
		info.ListenAddr = u.Webserver.bindAddr()
		info.URLBase = u.Webserver.URLBase
		info.Auth = u.uiPassword().Type().String()
		info.Metrics = u.Webserver.Metrics
		info.ConfigFile = u.ConfigFile
		info.Logs = u.logFileFolders()

		return nil
	})
	if err != nil {
		writeJSON(response, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, info)
}

func (u *Unpackerr) configHelpHandler(response http.ResponseWriter, _ *http.Request) {
	schema, err := configdef.Load()
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, schema.UIHelp())
}

func (u *Unpackerr) systemExportHandler(response http.ResponseWriter, request *http.Request) {
	info, _ := request.Context().Value(authCtxKey).(authInfo)

	var text string

	err := u.onMainLoop(request.Context(), func() error {
		text = u.liveConfigText(info)
		return nil
	})
	if err != nil {
		writeJSON(response, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"text": text})
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
		// Login secret stays blank even for *. Other secrets are visible to *.
		if envAlwaysRedact(key) || (!star && envValueSecret(key)) {
			out[key] = ""
			continue
		}

		out[key] = val
	}

	return out
}
