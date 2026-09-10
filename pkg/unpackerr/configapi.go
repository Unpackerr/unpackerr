package unpackerr

import (
	"net/http"

	"golift.io/cnfg"
)

// generalConfig is the top-level Config fields that are not nested app lists.
type generalConfig struct {
	Debug         bool          `json:"debug"`
	Quiet         bool          `json:"quiet"`
	Activity      bool          `json:"activity"`
	Parallel      uint          `json:"parallel"`
	ErrorStdErr   bool          `json:"errorStderr"`
	LogFile       string        `json:"logFile"`
	LogFiles      int           `json:"logFiles"`
	LogFileMb     int           `json:"logFileMb"`
	LogFileMode   string        `json:"logFileMode"`
	MaxRetries    uint          `json:"maxRetries"`
	RemnantAction string        `json:"remnantAction"`
	FileMode      string        `json:"fileMode"`
	DirMode       string        `json:"dirMode"`
	LogQueues     cnfg.Duration `json:"logQueues"`
	Interval      cnfg.Duration `json:"interval"`
	Timeout       cnfg.Duration `json:"timeout"`
	DeleteDelay   cnfg.Duration `json:"deleteDelay"`
	StartDelay    cnfg.Duration `json:"startDelay"`
	RetryDelay    cnfg.Duration `json:"retryDelay"`
	Progress      cnfg.Duration `json:"progress"`
	KeepHistory   uint          `json:"keepHistory"`
	Passwords     StringSlice   `json:"passwords"`
}

// foldersConfigAPI is global folder poller settings plus the watch list.
type foldersConfigAPI struct {
	Interval cnfg.Duration   `json:"interval"`
	Buffer   uint            `json:"buffer"`
	Folder   []*FolderConfig `json:"folder"`
}

func (u *Unpackerr) requireConfigPerm(write bool, next http.HandlerFunc) http.HandlerFunc {
	return u.requireAuth(func(response http.ResponseWriter, request *http.Request) {
		section := ConfigSection(request.PathValue("section"))
		if !KnownSection(section) {
			writeJSON(response, http.StatusNotFound, map[string]string{"error": "unknown section"})
			return
		}

		perm := PermReadConfig(section)
		if write {
			perm = PermWriteConfig(section)
		}

		info, _ := request.Context().Value(authCtxKey).(authInfo)
		if !info.allows(perm) {
			writeJSON(response, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		next(response, request)
	})
}

func (u *Unpackerr) configGetHandler(response http.ResponseWriter, request *http.Request) {
	section := ConfigSection(request.PathValue("section"))
	if section == SectionWebserver {
		web := publicWebserver(u.cloneFileWebserver())
		writeJSON(response, http.StatusOK, redactAPIKeysUnlessAll(request, web))

		return
	}

	// fileConfig is snapshotted in unmarshalConfig whether or not a file was found.
	u.configMu.RLock()
	cfg := cloneConfig(u.fileConfig)
	u.configMu.RUnlock()

	payload := configSectionFrom(cfg, section)
	writeJSON(response, http.StatusOK, payload)
}

func (u *Unpackerr) configGetLiveHandler(response http.ResponseWriter, request *http.Request) {
	section := ConfigSection(request.PathValue("section"))
	if section == SectionWebserver {
		writeJSON(response, http.StatusOK, redactAPIKeysUnlessAll(request, publicWebserver(u.cloneLiveWebserver())))
		return
	}

	// Live Config belongs to the main loop; snapshot it there.
	var payload any

	err := u.onMainLoop(request.Context(), func() error {
		if section == SectionGeneral {
			payload = u.liveGeneralConfig()
			return nil
		}

		payload = configSectionFrom(cloneConfig(u.Config), section)

		return nil
	})
	if err != nil {
		writeJSON(response, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, payload)
}

// configSectionFrom picks one section. requireConfigPerm already 404s unknown names.
func configSectionFrom(cfg *Config, section ConfigSection) any {
	switch section {
	case SectionGeneral:
		return generalConfigFrom(cfg)
	case SectionWebserver:
		if cfg.Webserver == nil {
			return &WebServer{}
		}

		return cfg.Webserver
	case SectionSonarr:
		return emptyIfNil(cfg.Sonarr)
	case SectionRadarr:
		return emptyIfNil(cfg.Radarr)
	case SectionLidarr:
		return emptyIfNil(cfg.Lidarr)
	case SectionReadarr:
		return emptyIfNil(cfg.Readarr)
	case SectionWhisparr:
		return emptyIfNil(cfg.Whisparr)
	case SectionFolders:
		return foldersConfigFrom(cfg)
	case SectionWebhooks:
		return emptyIfNil(cfg.Webhook)
	case SectionCmdhooks:
		return emptyIfNil(cfg.Cmdhook)
	default:
		return nil
	}
}

func generalConfigFrom(cfg *Config) generalConfig {
	return generalConfig{
		Debug:         cfg.Debug,
		Quiet:         cfg.Quiet,
		Activity:      cfg.Activity,
		Parallel:      cfg.Parallel,
		ErrorStdErr:   cfg.ErrorStdErr,
		LogFile:       cfg.LogFile,
		LogFiles:      cfg.LogFiles,
		LogFileMb:     cfg.LogFileMb,
		LogFileMode:   cfg.LogFileMode,
		MaxRetries:    cfg.MaxRetries,
		RemnantAction: cfg.RemnantAction,
		FileMode:      cfg.FileMode,
		DirMode:       cfg.DirMode,
		LogQueues:     cfg.LogQueues,
		Interval:      cfg.Interval,
		Timeout:       cfg.Timeout,
		DeleteDelay:   cfg.DeleteDelay,
		StartDelay:    cfg.StartDelay,
		RetryDelay:    cfg.RetryDelay,
		Progress:      cfg.Progress,
		KeepHistory:   cfg.KeepHistory,
		Passwords:     emptyIfNil(cfg.Passwords),
	}
}

func (u *Unpackerr) liveGeneralConfig() generalConfig {
	cfg := generalConfigFrom(u.Config)
	if u.livePasswords != nil {
		cfg.Passwords = emptyIfNil(append(StringSlice(nil), u.livePasswords...))
	}

	return cfg
}

// redactAPIKeysUnlessAll blanks webserver API key secrets unless the caller has *.
// Starr API keys are not webserver.APIKeys and stay visible to config:*:read.
func redactAPIKeysUnlessAll(request *http.Request, web *WebServer) *WebServer {
	info, _ := request.Context().Value(authCtxKey).(authInfo)
	if web == nil || info.allows(PermAll) {
		return web
	}

	for idx := range web.APIKeys {
		web.APIKeys[idx].Key = ""
	}

	return web
}

func publicWebserver(web *WebServer) *WebServer {
	if web == nil {
		return web
	}

	web.UIPassword = publicCryptPass(web.UIPassword)

	return web
}

func foldersConfigFrom(cfg *Config) foldersConfigAPI {
	return foldersConfigAPI{
		Interval: cfg.Folder.Interval,
		Buffer:   cfg.Folder.Buffer,
		Folder:   emptyIfNil(cfg.Folders),
	}
}

func emptyIfNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}

	return list
}
