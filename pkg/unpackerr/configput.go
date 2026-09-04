package unpackerr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"golift.io/starr"
	"golift.io/starr/lidarr"
	"golift.io/starr/radarr"
	"golift.io/starr/readarr"
	"golift.io/starr/sonarr"
)

const maxConfigBody = 1 << 20

var errInvalidJSON = errors.New("invalid json")

type configWriteReply struct {
	Status          string `json:"status"`
	RestartRequired bool   `json:"restartRequired"`
}

func (u *Unpackerr) configPutHandler(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	restart, err := u.replaceConfigSection(ConfigSection(params.ByName("section")), request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := u.writeConfigFile(); err != nil && !errors.Is(err, errNoConfigFile) {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, configWriteReply{Status: "ok", RestartRequired: restart})
}

func (u *Unpackerr) replaceConfigSection(section ConfigSection, request *http.Request) (bool, error) {
	switch section {
	case SectionGeneral:
		return u.putGeneral(request)
	case SectionWebserver:
		return u.putWebserver(request)
	case SectionSonarr:
		return false, u.putSonarr(request)
	case SectionRadarr:
		return false, u.putRadarr(request)
	case SectionLidarr:
		return false, u.putLidarr(request)
	case SectionReadarr:
		return false, u.putReadarr(request)
	case SectionWhisparr:
		return false, u.putWhisparr(request)
	case SectionFolders:
		return u.putFolders(request)
	case SectionWebhooks:
		return false, u.putWebhooks(request)
	case SectionCmdhooks:
		return false, u.putCmdhooks(request)
	default:
		return false, errInvalidJSON
	}
}

func readJSONBody(request *http.Request, dest any) error {
	if err := json.NewDecoder(io.LimitReader(request.Body, maxConfigBody)).Decode(dest); err != nil {
		return fmt.Errorf("%w: %w", errInvalidJSON, err)
	}

	return nil
}

func (u *Unpackerr) putGeneral(request *http.Request) (bool, error) {
	var next generalConfig
	if err := readJSONBody(request, &next); err != nil {
		return false, err
	}

	restart := next.Interval != u.Interval ||
		next.StartDelay != u.StartDelay ||
		next.Progress != u.Progress ||
		next.LogQueues != u.LogQueues ||
		next.Parallel != u.Parallel

	applyGeneral(u.Config, next)
	applyGeneral(u.fileConfig, next)
	u.validateConfig()

	if err := u.validateRemnantAction(); err != nil {
		return restart, err
	}

	if err := u.setPasswords(); err != nil {
		return restart, err
	}

	return restart, nil
}

func (u *Unpackerr) putWebserver(request *http.Request) (bool, error) {
	var next WebServer
	if err := readJSONBody(request, &next); err != nil {
		return false, err
	}

	if u.Webserver != nil {
		next.router = u.Webserver.router
		next.server = u.Webserver.server
		next.cookies = u.Webserver.cookies
		next.failDelay = u.Webserver.failDelay
	}

	omitted := next.UIPassword.Val() == ""

	submitted, fromFile, err := u.prepareWebserverPassword(&next)
	if err != nil {
		return false, err
	}

	if err := next.validateAuth(); err != nil {
		return false, err
	}

	next.allow = MakeIPs(next.Upstreams)

	restart := u.Webserver.ListenAddr != next.ListenAddr ||
		u.Webserver.URLBase != next.URLBase ||
		u.Webserver.SSLCrtFile != next.SSLCrtFile ||
		u.Webserver.SSLKeyFile != next.SSLKeyFile ||
		u.Webserver.Metrics != next.Metrics ||
		u.Webserver.Pprof != next.Pprof

	u.Webserver = &next
	u.storeFileWebserver(&next, submitted, fromFile, omitted)

	return restart, nil
}

func (u *Unpackerr) prepareWebserverPassword(next *WebServer) (CryptPass, bool, error) {
	submitted := next.UIPassword
	fromFile := strings.HasPrefix(submitted.Val(), filePrefix)

	if submitted.Val() == "" && u.Webserver != nil {
		next.UIPassword = u.Webserver.UIPassword
		return submitted, false, nil
	}

	if err := expandCryptPassFile(&next.UIPassword); err != nil {
		return submitted, fromFile, err
	}

	if err := normalizeStoredPassword(&next.UIPassword, u.uiPasswordUser()); err != nil {
		return submitted, fromFile, err
	}

	return submitted, fromFile, nil
}

func (u *Unpackerr) storeFileWebserver(live *WebServer, submitted CryptPass, fromFile, omitted bool) {
	if u.fileConfig == nil {
		return
	}

	cloned := cloneWebserver(live)

	switch {
	case omitted && u.fileConfig.Webserver != nil:
		cloned.UIPassword = u.fileConfig.Webserver.UIPassword
	case fromFile:
		cloned.UIPassword = submitted
	}

	u.fileConfig.Webserver = cloned
}

func applyGeneral(dst *Config, next generalConfig) {
	if dst == nil {
		return
	}

	dst.Debug = next.Debug
	dst.Quiet = next.Quiet
	dst.Activity = next.Activity
	dst.Parallel = next.Parallel
	dst.ErrorStdErr = next.ErrorStdErr
	dst.LogFile = next.LogFile
	dst.LogFiles = next.LogFiles
	dst.LogFileMb = next.LogFileMb
	dst.LogFileMode = next.LogFileMode
	dst.MaxRetries = next.MaxRetries
	dst.RemnantAction = next.RemnantAction
	dst.FileMode = next.FileMode
	dst.DirMode = next.DirMode
	dst.LogQueues = next.LogQueues
	dst.Interval = next.Interval
	dst.Timeout = next.Timeout
	dst.DeleteDelay = next.DeleteDelay
	dst.StartDelay = next.StartDelay
	dst.RetryDelay = next.RetryDelay
	dst.Progress = next.Progress
	dst.KeepHistory = next.KeepHistory
	dst.Passwords = append(StringSlice(nil), next.Passwords...)
}

func (u *Unpackerr) uiPasswordUser() string {
	if u.Webserver != nil {
		if name := u.Webserver.UIPassword.Username(); name != "" {
			return name
		}
	}

	return defaultUIUser
}

func normalizeStoredPassword(pass *CryptPass, fallback string) error {
	if pass.Val() == "" || pass.IsCrypted() || pass.Webauth() {
		return nil
	}

	user, plain := splitUserPass(pass.Val(), fallback)

	return pass.SetPlain(user, plain)
}

func (u *Unpackerr) putSonarr(request *http.Request) error {
	var list []*SonarrConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Sonarr); err != nil {
			return err
		}

		list[idx].Sonarr = sonarr.New(&list[idx].Config)
	}

	u.Sonarr = list

	if u.fileConfig != nil {
		u.fileConfig.Sonarr = cloneSonarrList(list)
	}

	return nil
}

func (u *Unpackerr) putRadarr(request *http.Request) error {
	var list []*RadarrConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Radarr); err != nil {
			return err
		}

		list[idx].Radarr = radarr.New(&list[idx].Config)
	}

	u.Radarr = list

	if u.fileConfig != nil {
		u.fileConfig.Radarr = cloneRadarrList(list)
	}

	return nil
}

func (u *Unpackerr) putLidarr(request *http.Request) error {
	var list []*LidarrConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Lidarr); err != nil {
			return err
		}

		list[idx].Lidarr = lidarr.New(&list[idx].Config)
	}

	u.Lidarr = list

	if u.fileConfig != nil {
		u.fileConfig.Lidarr = cloneLidarrList(list)
	}

	return nil
}

func (u *Unpackerr) putReadarr(request *http.Request) error {
	var list []*ReadarrConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Readarr); err != nil {
			return err
		}

		list[idx].Readarr = readarr.New(&list[idx].Config)
	}

	u.Readarr = list

	if u.fileConfig != nil {
		u.fileConfig.Readarr = cloneReadarrList(list)
	}

	return nil
}

func (u *Unpackerr) putWhisparr(request *http.Request) error {
	var list []*RadarrConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Whisparr); err != nil {
			return err
		}

		list[idx].Radarr = radarr.New(&list[idx].Config)
	}

	u.Whisparr = list

	if u.fileConfig != nil {
		u.fileConfig.Whisparr = cloneRadarrList(list)
	}

	return nil
}

func (u *Unpackerr) putFolders(request *http.Request) (bool, error) {
	var next foldersConfigAPI
	if err := readJSONBody(request, &next); err != nil {
		return true, err
	}

	u.Folder.Interval = next.Interval
	u.Folder.Buffer = next.Buffer
	u.Folders = next.Folder

	if u.fileConfig != nil {
		u.fileConfig.Folder.Interval = next.Interval
		u.fileConfig.Folder.Buffer = next.Buffer
		u.fileConfig.Folders = cloneFolderList(next.Folder)
	}

	return true, u.validateFolders()
}

func (u *Unpackerr) putWebhooks(request *http.Request) error {
	var list []*WebhookConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	u.Webhook = list

	if u.fileConfig != nil {
		u.fileConfig.Webhook = cloneHookList(list)
	}

	return u.validateWebhook()
}

func (u *Unpackerr) putCmdhooks(request *http.Request) error {
	var list []*WebhookConfig
	if err := readJSONBody(request, &list); err != nil {
		return err
	}

	u.Cmdhook = list

	if u.fileConfig != nil {
		u.fileConfig.Cmdhook = cloneHookList(list)
	}

	return u.validateCmdhook()
}
