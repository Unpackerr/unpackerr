package unpackerr

import (
	"bytes"
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

var (
	errInvalidJSON          = errors.New("invalid json")
	errEmptyConfigSection   = errors.New("empty config section")
	errNilConfigEntry       = errors.New("nil config entry")
	errUnknownConfigSection = errors.New("unknown section")
	errPersistConfig        = errors.New("persisting config file")
)

type configWriteReply struct {
	Status          string `json:"status"`
	RestartRequired bool   `json:"restartRequired"`
}

func (u *Unpackerr) configPutHandler(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	restart, err := u.replaceConfigSection(ConfigSection(params.ByName("section")), response, request)
	if err != nil {
		writeJSON(response, statusForConfigPut(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, configWriteReply{Status: "ok", RestartRequired: restart})
}

func statusForConfigPut(err error) int {
	if errors.Is(err, errPersistConfig) || errors.Is(err, errNoFileSnapshot) {
		return http.StatusInternalServerError
	}

	return http.StatusBadRequest
}

func (u *Unpackerr) replaceConfigSection(
	section ConfigSection,
	response http.ResponseWriter,
	request *http.Request,
) (bool, error) {
	switch section {
	case SectionGeneral:
		return u.putGeneral(response, request)
	case SectionWebserver:
		return u.putWebserver(response, request)
	case SectionSonarr:
		return false, u.putSonarr(response, request)
	case SectionRadarr:
		return false, u.putRadarr(response, request)
	case SectionLidarr:
		return false, u.putLidarr(response, request)
	case SectionReadarr:
		return false, u.putReadarr(response, request)
	case SectionWhisparr:
		return false, u.putWhisparr(response, request)
	case SectionFolders:
		return u.putFolders(response, request)
	case SectionWebhooks:
		return false, u.putWebhooks(response, request)
	case SectionCmdhooks:
		return false, u.putCmdhooks(response, request)
	default:
		return false, errUnknownConfigSection
	}
}

func decodeJSONBody(response http.ResponseWriter, request *http.Request) (json.RawMessage, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxConfigBody))

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, wrapJSONErr(err)
	}

	switch err := decoder.Decode(&struct{}{}); {
	case errors.Is(err, io.EOF):
	case err != nil:
		return nil, wrapJSONErr(err)
	default:
		return nil, errExtraJSON
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, errEmptyConfigSection
	}

	return raw, nil
}

func unmarshalConfigJSON(raw json.RawMessage, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dest); err != nil {
		return wrapJSONErr(err)
	}

	return nil
}

func wrapJSONErr(err error) error {
	return fmt.Errorf("%w: %w", errInvalidJSON, err)
}

func readJSONBody(response http.ResponseWriter, request *http.Request, dest any) error {
	raw, err := decodeJSONBody(response, request)
	if err != nil {
		return err
	}

	return unmarshalConfigJSON(raw, dest)
}

func readJSONObject(response http.ResponseWriter, request *http.Request, dest any) error {
	raw, err := decodeJSONBody(response, request)
	if err != nil {
		return err
	}

	if bytes.Equal(bytes.TrimSpace(raw), []byte("{}")) {
		return errEmptyConfigSection
	}

	return unmarshalConfigJSON(raw, dest)
}

func rejectNilPointers[T any](list []*T) error {
	for _, item := range list {
		if item == nil {
			return errNilConfigEntry
		}
	}

	return nil
}

// commitConfig writes a staged file snapshot, then publishes live state.
// A write failure leaves live and fileConfig unchanged. Env-only (no config
// path) still applies live.
func (u *Unpackerr) commitConfig(mutateFile func(*Config), applyLive func()) error {
	u.configMu.Lock()
	defer u.configMu.Unlock()

	if u.fileConfig != nil {
		staged := cloneConfig(u.fileConfig)
		mutateFile(staged)

		if err := u.writeConfigFrom(staged); err != nil && !errors.Is(err, errNoConfigFile) {
			return err
		}

		u.fileConfig = staged
	}

	applyLive()

	return nil
}

func (u *Unpackerr) putGeneral(response http.ResponseWriter, request *http.Request) (bool, error) {
	var next generalConfig
	if err := readJSONObject(response, request, &next); err != nil {
		return false, err
	}

	if err := remnantActionError(next.RemnantAction); err != nil {
		return false, err
	}

	expanded, err := expandPasswords(next.Passwords)
	if err != nil {
		return false, err
	}

	submittedPasswords := append(StringSlice(nil), next.Passwords...)
	restart := generalRestartRequired(u.Config, next)

	return restart, u.commitConfig(func(cfg *Config) {
		applyGeneral(cfg, next)
	}, func() {
		applyGeneral(u.Config, next)
		u.livePasswords = submittedPasswords
		u.Passwords = expanded
		u.RemnantAction = remnantAction(next.RemnantAction)
		u.validateConfig()
	})
}

func generalRestartRequired(cur *Config, next generalConfig) bool {
	if cur == nil {
		return true
	}

	return next.Interval != cur.Interval ||
		next.StartDelay != cur.StartDelay ||
		next.Progress != cur.Progress ||
		next.LogQueues != cur.LogQueues ||
		next.Parallel != cur.Parallel ||
		next.LogFile != cur.LogFile ||
		next.LogFiles != cur.LogFiles ||
		next.LogFileMb != cur.LogFileMb ||
		next.LogFileMode != cur.LogFileMode ||
		next.ErrorStdErr != cur.ErrorStdErr ||
		next.FileMode != cur.FileMode ||
		next.DirMode != cur.DirMode
}

func (u *Unpackerr) putWebserver(response http.ResponseWriter, request *http.Request) (bool, error) {
	var next WebServer
	if err := readJSONObject(response, request, &next); err != nil {
		return false, err
	}

	next.normalizeURLBase()

	prevFile := u.cloneStoredFileWebserver()
	keepNamedAPIKeys(&next, u.Webserver, prevFile)

	omitted := next.UIPassword.Val() == ""

	submitted, fromFile, err := u.prepareWebserverPassword(&next)
	if err != nil {
		return false, err
	}

	if err := next.validateAuth(); err != nil {
		return false, err
	}

	next.allow = MakeIPs(next.Upstreams)
	restart := webserverRestartRequired(u.Webserver, &next)
	fileWeb := fileWebserverFromPut(&next, submitted, fromFile, omitted, prevFile)

	if err := u.commitConfig(func(cfg *Config) {
		cfg.Webserver = fileWeb
	}, func() {}); err != nil {
		return false, err
	}

	u.applyLiveWebserverAuth(&next)

	return restart, nil
}

func webserverRestartRequired(cur, next *WebServer) bool {
	if cur == nil || next == nil {
		return true
	}

	return cur.ListenAddr != next.ListenAddr ||
		cur.URLBase != next.URLBase ||
		cur.SSLCrtFile != next.SSLCrtFile ||
		cur.SSLKeyFile != next.SSLKeyFile ||
		cur.Metrics != next.Metrics ||
		cur.Pprof != next.Pprof ||
		cur.LogFile != next.LogFile ||
		cur.LogFiles != next.LogFiles ||
		cur.LogFileMb != next.LogFileMb
}

func fileWebserverFromPut(
	next *WebServer,
	submitted CryptPass,
	fromFile, omitted bool,
	prevFile *WebServer,
) *WebServer {
	cloned := cloneWebserver(next)

	switch {
	case omitted && prevFile != nil:
		cloned.UIPassword = prevFile.UIPassword
	case fromFile:
		cloned.UIPassword = submitted
	}

	return cloned
}

// keepNamedAPIKeys fills a blank apiKeys[].key from an existing key of the same
// name so a redacted GET can round-trip without requiring PermAll.
func keepNamedAPIKeys(next *WebServer, sources ...*WebServer) {
	if next == nil {
		return
	}

	byName := make(map[string]string)

	for _, src := range sources {
		if src == nil {
			continue
		}

		for idx := range src.APIKeys {
			name := src.APIKeys[idx].Name
			if name == "" || src.APIKeys[idx].Key == "" {
				continue
			}

			if _, exists := byName[name]; !exists {
				byName[name] = src.APIKeys[idx].Key
			}
		}
	}

	for idx := range next.APIKeys {
		if strings.TrimSpace(next.APIKeys[idx].Key) != "" {
			continue
		}

		next.APIKeys[idx].Key = byName[next.APIKeys[idx].Name]
	}
}

func (u *Unpackerr) applyLiveWebserverAuth(next *WebServer) {
	if u == nil || u.Webserver == nil || next == nil {
		return
	}

	u.uiPassMu.Lock()
	defer u.uiPassMu.Unlock()

	u.Webserver.APIKeys = cloneAPIKeys(next.APIKeys)
	u.Webserver.Roles = cloneRoles(next.Roles)
	u.Webserver.UIPassword = next.UIPassword
	u.Webserver.Upstreams = append(StringSlice(nil), next.Upstreams...)
	u.Webserver.allow = next.allow
	u.Webserver.keyPerms = next.keyPerms
}

func (u *Unpackerr) prepareWebserverPassword(next *WebServer) (CryptPass, bool, error) {
	submitted := next.UIPassword
	fromFile := strings.HasPrefix(submitted.Val(), filePrefix)

	if submitted.Val() == "" && u.Webserver != nil {
		next.UIPassword = u.uiPassword()

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
	if name := u.uiPassword().Username(); name != "" {
		return name
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

func (u *Unpackerr) putSonarr(response http.ResponseWriter, request *http.Request) error {
	var list []*SonarrConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := rejectNilPointers(list); err != nil {
		return err
	}

	fileList := cloneSonarrList(list)

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Sonarr); err != nil {
			return err
		}

		list[idx].Sonarr = sonarr.New(&list[idx].Config)
	}

	carrySonarrQueues(u.Sonarr, list)

	return u.commitConfig(func(cfg *Config) {
		cfg.Sonarr = fileList
	}, func() {
		u.Sonarr = list
	})
}

func (u *Unpackerr) putRadarr(response http.ResponseWriter, request *http.Request) error {
	var list []*RadarrConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := rejectNilPointers(list); err != nil {
		return err
	}

	fileList := cloneRadarrList(list)

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Radarr); err != nil {
			return err
		}

		list[idx].Radarr = radarr.New(&list[idx].Config)
	}

	carryRadarrQueues(u.Radarr, list)

	return u.commitConfig(func(cfg *Config) {
		cfg.Radarr = fileList
	}, func() {
		u.Radarr = list
	})
}

func (u *Unpackerr) putLidarr(response http.ResponseWriter, request *http.Request) error {
	var list []*LidarrConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := rejectNilPointers(list); err != nil {
		return err
	}

	fileList := cloneLidarrList(list)

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Lidarr); err != nil {
			return err
		}

		list[idx].Lidarr = lidarr.New(&list[idx].Config)
	}

	carryLidarrQueues(u.Lidarr, list)

	return u.commitConfig(func(cfg *Config) {
		cfg.Lidarr = fileList
	}, func() {
		u.Lidarr = list
	})
}

func (u *Unpackerr) putReadarr(response http.ResponseWriter, request *http.Request) error {
	var list []*ReadarrConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := rejectNilPointers(list); err != nil {
		return err
	}

	fileList := cloneReadarrList(list)

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Readarr); err != nil {
			return err
		}

		list[idx].Readarr = readarr.New(&list[idx].Config)
	}

	carryReadarrQueues(u.Readarr, list)

	return u.commitConfig(func(cfg *Config) {
		cfg.Readarr = fileList
	}, func() {
		u.Readarr = list
	})
}

func (u *Unpackerr) putWhisparr(response http.ResponseWriter, request *http.Request) error {
	var list []*RadarrConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := rejectNilPointers(list); err != nil {
		return err
	}

	fileList := cloneRadarrList(list)

	for idx := range list {
		if err := u.validateApp(&list[idx].StarrConfig, starr.Whisparr); err != nil {
			return err
		}

		list[idx].Radarr = radarr.New(&list[idx].Config)
	}

	carryRadarrQueues(u.Whisparr, list)

	return u.commitConfig(func(cfg *Config) {
		cfg.Whisparr = fileList
	}, func() {
		u.Whisparr = list
	})
}

func starrIdentity(url, apiKey string) string {
	return url + "\x00" + apiKey
}

func carrySonarrQueues(prev, next []*SonarrConfig) {
	seen := make(map[string]*SonarrConfig, len(prev))

	for _, app := range prev {
		if app != nil {
			seen[starrIdentity(app.URL, app.APIKey)] = app
		}
	}

	for _, app := range next {
		if app == nil {
			continue
		}

		if old := seen[starrIdentity(app.URL, app.APIKey)]; old != nil {
			app.Queue = old.Queue
		}
	}
}

func carryRadarrQueues(prev, next []*RadarrConfig) {
	seen := make(map[string]*RadarrConfig, len(prev))

	for _, app := range prev {
		if app != nil {
			seen[starrIdentity(app.URL, app.APIKey)] = app
		}
	}

	for _, app := range next {
		if app == nil {
			continue
		}

		if old := seen[starrIdentity(app.URL, app.APIKey)]; old != nil {
			app.Queue = old.Queue
		}
	}
}

func carryLidarrQueues(prev, next []*LidarrConfig) {
	seen := make(map[string]*LidarrConfig, len(prev))

	for _, app := range prev {
		if app != nil {
			seen[starrIdentity(app.URL, app.APIKey)] = app
		}
	}

	for _, app := range next {
		if app == nil {
			continue
		}

		if old := seen[starrIdentity(app.URL, app.APIKey)]; old != nil {
			app.Queue = old.Queue
		}
	}
}

func carryReadarrQueues(prev, next []*ReadarrConfig) {
	seen := make(map[string]*ReadarrConfig, len(prev))

	for _, app := range prev {
		if app != nil {
			seen[starrIdentity(app.URL, app.APIKey)] = app
		}
	}

	for _, app := range next {
		if app == nil {
			continue
		}

		if old := seen[starrIdentity(app.URL, app.APIKey)]; old != nil {
			app.Queue = old.Queue
		}
	}
}

func (u *Unpackerr) putFolders(response http.ResponseWriter, request *http.Request) (bool, error) {
	var next foldersConfigAPI
	if err := readJSONObject(response, request, &next); err != nil {
		return false, err
	}

	if err := rejectNilPointers(next.Folder); err != nil {
		return false, err
	}

	if err := validateFolderList(next.Folder); err != nil {
		return false, err
	}

	fileList := cloneFolderList(next.Folder)

	return true, u.commitConfig(func(cfg *Config) {
		cfg.Folder.Interval = next.Interval
		cfg.Folder.Buffer = next.Buffer
		cfg.Folders = fileList
	}, func() {
		u.Folder.Interval = next.Interval
		u.Folder.Buffer = next.Buffer
		u.Folders = next.Folder
	})
}

func (u *Unpackerr) putWebhooks(response http.ResponseWriter, request *http.Request) error {
	var list []*WebhookConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := u.validateWebhookList(list); err != nil {
		return err
	}

	fileList := cloneHookList(list)

	if err := u.commitConfig(func(cfg *Config) {
		cfg.Webhook = fileList
	}, func() {
		u.Webhook = list
	}); err != nil {
		return err
	}

	u.ensureHookWorker()

	return nil
}

func (u *Unpackerr) putCmdhooks(response http.ResponseWriter, request *http.Request) error {
	var list []*WebhookConfig
	if err := readJSONBody(response, request, &list); err != nil {
		return err
	}

	if err := u.validateCmdhookList(list); err != nil {
		return err
	}

	fileList := cloneHookList(list)

	if err := u.commitConfig(func(cfg *Config) {
		cfg.Cmdhook = fileList
	}, func() {
		u.Cmdhook = list
	}); err != nil {
		return err
	}

	u.ensureHookWorker()

	return nil
}
