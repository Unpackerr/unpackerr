package unpackerr

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
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

func (u *Unpackerr) requireConfigPerm(write bool, next httprouter.Handle) httprouter.Handle {
	return u.requireAuth(func(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
		section := ConfigSection(params.ByName("section"))
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

		next(response, request, params)
	})
}

func (u *Unpackerr) configGetHandler(response http.ResponseWriter, _ *http.Request, params httprouter.Params) {
	writeConfigSection(response, u.fileConfigOrLive(), ConfigSection(params.ByName("section")))
}

func (u *Unpackerr) configGetLiveHandler(response http.ResponseWriter, _ *http.Request, params httprouter.Params) {
	writeConfigSection(response, u.Config, ConfigSection(params.ByName("section")))
}

func (u *Unpackerr) fileConfigOrLive() *Config {
	if u.fileConfig != nil {
		return u.fileConfig
	}

	return u.Config
}

func writeConfigSection(response http.ResponseWriter, cfg *Config, section ConfigSection) {
	payload, ok := configSectionFrom(cfg, section)
	if !ok {
		writeJSON(response, http.StatusNotFound, map[string]string{"error": "unknown section"})
		return
	}

	writeJSON(response, http.StatusOK, payload)
}

func configSectionFrom(cfg *Config, section ConfigSection) (any, bool) {
	if cfg == nil {
		return nil, false
	}

	switch section {
	case SectionGeneral:
		return generalConfigFrom(cfg), true
	case SectionWebserver:
		if cfg.Webserver == nil {
			return &WebServer{}, true
		}

		return cfg.Webserver, true
	case SectionSonarr:
		return emptyIfNil(cfg.Sonarr), true
	case SectionRadarr:
		return emptyIfNil(cfg.Radarr), true
	case SectionLidarr:
		return emptyIfNil(cfg.Lidarr), true
	case SectionReadarr:
		return emptyIfNil(cfg.Readarr), true
	case SectionWhisparr:
		return emptyIfNil(cfg.Whisparr), true
	case SectionFolders:
		return foldersConfigFrom(cfg), true
	case SectionWebhooks:
		return emptyIfNil(cfg.Webhook), true
	case SectionCmdhooks:
		return emptyIfNil(cfg.Cmdhook), true
	default:
		return nil, false
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
