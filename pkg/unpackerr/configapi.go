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
	payload, ok := u.configSection(ConfigSection(params.ByName("section")))
	if !ok {
		writeJSON(response, http.StatusNotFound, map[string]string{"error": "unknown section"})
		return
	}

	writeJSON(response, http.StatusOK, payload)
}

func (u *Unpackerr) configSection(section ConfigSection) (any, bool) {
	switch section {
	case SectionGeneral:
		return u.generalConfig(), true
	case SectionWebserver:
		return u.Webserver, true
	case SectionSonarr:
		return emptyIfNil(u.Sonarr), true
	case SectionRadarr:
		return emptyIfNil(u.Radarr), true
	case SectionLidarr:
		return emptyIfNil(u.Lidarr), true
	case SectionReadarr:
		return emptyIfNil(u.Readarr), true
	case SectionWhisparr:
		return emptyIfNil(u.Whisparr), true
	case SectionFolders:
		return u.foldersConfig(), true
	case SectionWebhooks:
		return emptyIfNil(u.Webhook), true
	case SectionCmdhooks:
		return emptyIfNil(u.Cmdhook), true
	default:
		return nil, false
	}
}

func (u *Unpackerr) generalConfig() generalConfig {
	return generalConfig{
		Debug:         u.Config.Debug,
		Quiet:         u.Quiet,
		Activity:      u.Activity,
		Parallel:      u.Parallel,
		ErrorStdErr:   u.ErrorStdErr,
		LogFile:       u.LogFile,
		LogFiles:      u.LogFiles,
		LogFileMb:     u.LogFileMb,
		LogFileMode:   u.LogFileMode,
		MaxRetries:    u.MaxRetries,
		RemnantAction: u.RemnantAction,
		FileMode:      u.FileMode,
		DirMode:       u.DirMode,
		LogQueues:     u.LogQueues,
		Interval:      u.Interval,
		Timeout:       u.Timeout,
		DeleteDelay:   u.DeleteDelay,
		StartDelay:    u.StartDelay,
		RetryDelay:    u.RetryDelay,
		Progress:      u.Progress,
		KeepHistory:   u.KeepHistory,
		Passwords:     emptyIfNil(u.Passwords),
	}
}

func (u *Unpackerr) foldersConfig() foldersConfigAPI {
	return foldersConfigAPI{
		Interval: u.Folder.Interval,
		Buffer:   u.Folder.Buffer,
		Folder:   emptyIfNil(u.Folders),
	}
}

func emptyIfNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}

	return list
}
