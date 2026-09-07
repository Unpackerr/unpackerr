package unpackerr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"golift.io/cnfgfile"
	"golift.io/starr"
)

const maxConfigBody = 1 << 20

var (
	errInvalidJSON        = errors.New("invalid json")
	errEmptyConfigSection = errors.New("empty config section")
	errNilConfigEntry     = errors.New("nil config entry")
	errPersistConfig      = errors.New("persisting config file")
)

type configWriteReply struct {
	Status          string `json:"status"`
	RestartRequired bool   `json:"restartRequired"`
}

// configPutHandler reads the body on the HTTP goroutine, then decodes,
// validates, writes the file, and applies live on the main loop.
func (u *Unpackerr) configPutHandler(response http.ResponseWriter, request *http.Request, params httprouter.Params) {
	section := ConfigSection(params.ByName("section"))

	raw, err := decodeJSONBody(response, request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var restart bool

	err = u.onMainLoop(request.Context(), func() error { //nolint:contextcheck // hook worker outlives the request.
		var err error

		restart, err = u.replaceConfigSection(section, raw)
		if err != nil {
			return err // nothing changed, so nothing needs a restart.
		}

		u.pendingRestart = u.pendingRestart || restart

		return nil
	})
	if err != nil {
		writeJSON(response, statusForConfigPut(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, configWriteReply{Status: "ok", RestartRequired: restart})
}

func statusForConfigPut(err error) int {
	switch {
	case errors.Is(err, errPersistConfig):
		return http.StatusInternalServerError
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusBadRequest
	}
}

// replaceConfigSection runs on the main loop. Sections that need a process
// restart to fully apply return true; the caller sets pendingRestart.
func (u *Unpackerr) replaceConfigSection(section ConfigSection, raw json.RawMessage) (bool, error) {
	switch section {
	case SectionGeneral:
		return u.putGeneral(raw)
	case SectionWebserver:
		return u.putWebserver(raw)
	case SectionSonarr:
		return false, putStarrList(u, raw, starr.Sonarr, func(c *Config) *[]*SonarrConfig { return &c.Sonarr })
	case SectionRadarr:
		return false, putStarrList(u, raw, starr.Radarr, func(c *Config) *[]*RadarrConfig { return &c.Radarr })
	case SectionLidarr:
		return false, putStarrList(u, raw, starr.Lidarr, func(c *Config) *[]*LidarrConfig { return &c.Lidarr })
	case SectionReadarr:
		return false, putStarrList(u, raw, starr.Readarr, func(c *Config) *[]*ReadarrConfig { return &c.Readarr })
	case SectionWhisparr:
		return false, putStarrList(u, raw, starr.Whisparr, func(c *Config) *[]*RadarrConfig { return &c.Whisparr })
	case SectionFolders:
		return u.putFolders(raw)
	case SectionWebhooks:
		return false, u.putHooks(raw, u.validateWebhookList, func(c *Config) *[]*WebhookConfig { return &c.Webhook })
	case SectionCmdhooks:
		return false, u.putHooks(raw, u.validateCmdhookList, func(c *Config) *[]*WebhookConfig { return &c.Cmdhook })
	default:
		return false, fmt.Errorf("%w: %s", errUnknownSection, section)
	}
}

var errUnknownSection = errors.New("unknown section")

// decodeJSONBody reads one JSON value and rejects empty, null, or trailing data.
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

func wrapJSONErr(err error) error {
	return fmt.Errorf("%w: %w", errInvalidJSON, err)
}

// unmarshalStrict decodes with unknown fields rejected.
func unmarshalStrict(raw json.RawMessage, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dest); err != nil {
		return wrapJSONErr(err)
	}

	return nil
}

// unmarshalObject is unmarshalStrict plus a guard against a bare {}, which
// would otherwise zero every field in an object section.
func unmarshalObject(raw json.RawMessage, dest any) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return wrapJSONErr(err)
	}

	if len(probe) == 0 {
		return errEmptyConfigSection
	}

	return unmarshalStrict(raw, dest)
}

// unmarshalList is unmarshalStrict plus a guard against [null] entries.
func unmarshalList[T any, P interface{ *T }](raw json.RawMessage, dest *[]P) error {
	if err := unmarshalStrict(raw, dest); err != nil {
		return err
	}

	for _, item := range *dest {
		if item == nil {
			return errNilConfigEntry
		}
	}

	return nil
}

// expandFilepaths replaces filepath: values on a live copy, the same way
// startup does for the whole Config. The file-shaped copy keeps the prefix.
func expandFilepaths(ptr any) error {
	_, err := cnfgfile.Parse(ptr, &cnfgfile.Opts{
		Name:          "Unpackerr",
		TransformPath: expandHomedir,
		Prefix:        filePrefix,
	})
	if err != nil {
		return fmt.Errorf("parsing filepaths: %w", err)
	}

	return nil
}

// commitConfig stages the change onto a clone of fileConfig, writes the TOML,
// then publishes live. A failed write changes nothing. Runs on the main loop;
// configMu covers fileConfig and the hook slices that /api/stats reads.
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

func (u *Unpackerr) putGeneral(raw json.RawMessage) (bool, error) {
	var next generalConfig
	if err := unmarshalObject(raw, &next); err != nil {
		return false, err
	}

	if err := remnantActionError(next.RemnantAction); err != nil {
		return false, err
	}

	expanded, err := expandPasswords(next.Passwords)
	if err != nil {
		return false, err
	}

	// Decide the restart from a clamped copy. Comparing raw input against the
	// live config would ask for a restart whenever a value was omitted, since
	// clampConfig is about to fill it with the same default.
	staged := *u.Config
	applyGeneral(&staged, next)
	clampConfig(&staged)

	restart := generalRestartRequired(u.Config, &staged)
	historyWasOff := u.KeepHistory == 0

	return restart, u.commitConfig(func(cfg *Config) {
		applyGeneral(cfg, next)
	}, func() {
		applyGeneral(u.Config, next)
		u.livePasswords = append(StringSlice(nil), next.Passwords...)
		u.Passwords = expanded
		u.RemnantAction = remnantAction(next.RemnantAction)
		clampConfig(u.Config)
		u.ensureTrayRing()
		u.resetTickers()

		if historyWasOff && u.KeepHistory > 0 {
			u.loadHistory() // histPath is only resolved while history is enabled.
		}
	})
}

// generalRestartRequired lists the general fields the main loop cannot re-apply
// in place: logger construction, the xtractr instance, and the defaults that
// validation already copied into each Starr app and hook.
func generalRestartRequired(cur, next *Config) bool {
	return next.Debug != cur.Debug ||
		next.Quiet != cur.Quiet ||
		next.Parallel != cur.Parallel ||
		next.LogFile != cur.LogFile ||
		next.LogFiles != cur.LogFiles ||
		next.LogFileMb != cur.LogFileMb ||
		next.LogFileMode != cur.LogFileMode ||
		next.ErrorStdErr != cur.ErrorStdErr ||
		next.FileMode != cur.FileMode ||
		next.DirMode != cur.DirMode ||
		// Both only seed per-app values in validateApp and validate*HookList,
		// so running clients keep the old value until they are rebuilt.
		next.Timeout != cur.Timeout ||
		next.DeleteDelay != cur.DeleteDelay
}

func (u *Unpackerr) putWebserver(raw json.RawMessage) (bool, error) {
	var next WebServer
	if err := unmarshalObject(raw, &next); err != nil {
		return false, err
	}

	next.normalizeURLBase()

	submitted := next.UIPassword
	omitted := submitted.Val() == ""
	fromFile := strings.HasPrefix(submitted.Val(), filePrefix)

	if !omitted {
		if err := expandCryptPassFile(&next.UIPassword); err != nil {
			return false, err
		}

		if err := normalizeStoredPassword(&next.UIPassword, u.uiPasswordUser()); err != nil {
			return false, err
		}
	}

	liveSnap := u.cloneLiveWebserver()
	fileSnap := u.cloneFileWebserver()

	// Blank keys round-trip from a redacted GET. File keys fill from the file
	// only so an env-overlay key never lands on disk; live fills from live then file.
	fileOnly := &WebServer{APIKeys: cloneAPIKeys(next.APIKeys)}
	keepNamedAPIKeys(fileOnly, fileSnap)
	dropEmptyAPIKeys(fileOnly)
	keepNamedAPIKeys(&next, liveSnap, fileSnap)

	if omitted {
		next.UIPassword = liveSnap.UIPassword
	}

	if err := next.validateAuth(); err != nil {
		return false, err
	}

	next.allow = MakeIPs(next.Upstreams)

	fileWeb := cloneWebserver(&next)
	fileWeb.APIKeys = fileOnly.APIKeys

	switch {
	case omitted:
		fileWeb.UIPassword = fileSnap.UIPassword
	case fromFile:
		fileWeb.UIPassword = submitted
	}

	if err := fileWeb.validateAuth(); err != nil {
		return false, err
	}

	restart := webserverRestartRequired(liveSnap, &next)

	return restart, u.commitConfig(func(cfg *Config) {
		cfg.Webserver = fileWeb
	}, func() {
		u.applyLiveWebserverAuth(&next)
	})
}

func webserverRestartRequired(cur, next *WebServer) bool {
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

func dropEmptyAPIKeys(web *WebServer) {
	out := make([]APIKey, 0, len(web.APIKeys))

	for _, key := range web.APIKeys {
		if strings.TrimSpace(key.Key) != "" {
			out = append(out, key)
		}
	}

	web.APIKeys = out
}

// keepNamedAPIKeys fills a blank apiKeys[].key from an existing key of the
// same name so a redacted GET can round-trip without requiring PermAll.
func keepNamedAPIKeys(next *WebServer, sources ...*WebServer) {
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
		if strings.TrimSpace(next.APIKeys[idx].Key) == "" {
			next.APIKeys[idx].Key = byName[next.APIKeys[idx].Name]
		}
	}
}

// applyLiveWebserverAuth swaps the auth fields HTTP goroutines read.
// The listener itself is not touched; listen/TLS/urlbase need a restart.
func (u *Unpackerr) applyLiveWebserverAuth(next *WebServer) {
	u.uiPassMu.Lock()
	defer u.uiPassMu.Unlock()

	u.Webserver.APIKeys = cloneAPIKeys(next.APIKeys)
	u.Webserver.Roles = cloneRoles(next.Roles)
	u.Webserver.UIPassword = next.UIPassword
	u.Webserver.Upstreams = append(StringSlice(nil), next.Upstreams...)
	u.Webserver.allow = next.allow
	u.Webserver.keyPerms = next.keyPerms
}

func applyGeneral(dst *Config, next generalConfig) {
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

// putStarrList replaces one Starr app list. The file copy keeps filepath:
// values; the live copy is expanded, validated, and given clients. Queues
// carry over by url+apikey so an unchanged app keeps polling state.
func putStarrList[T any, P starrApp[T]](
	unpackerr *Unpackerr, raw json.RawMessage, app starr.App, field func(*Config) *[]P,
) error {
	var list []P
	if err := unmarshalList(raw, &list); err != nil {
		return err
	}

	fileList := cloneStarrList(list)

	if err := expandFilepaths(&list); err != nil {
		return err
	}

	for _, item := range list {
		if err := unpackerr.validateApp(item.conf(), app); err != nil {
			return err
		}

		item.connect()
	}

	return unpackerr.commitConfig(func(cfg *Config) {
		*field(cfg) = fileList
	}, func() {
		live := field(unpackerr.Config)
		carryQueues(*live, list)
		*live = list

		unpackerr.ensureWorkThreads(unpackerr.starrAppCount())
	})
}

func starrIdentity(conf *StarrConfig) string {
	return conf.URL + "\x00" + conf.APIKey
}

func carryQueues[T any, P starrApp[T]](prev, next []P) {
	seen := make(map[string]P, len(prev))
	for _, app := range prev {
		seen[starrIdentity(app.conf())] = app
	}

	for _, app := range next {
		if old, ok := seen[starrIdentity(app.conf())]; ok {
			app.takeQueue(old)
		}
	}
}

func (u *Unpackerr) putFolders(raw json.RawMessage) (bool, error) {
	var next foldersConfigAPI
	if err := unmarshalObject(raw, &next); err != nil {
		return false, err
	}

	for _, folder := range next.Folder {
		if folder == nil {
			return false, errNilConfigEntry
		}
	}

	fileList := cloneFolderList(next.Folder)

	if err := expandFilepaths(&next.Folder); err != nil {
		return false, err
	}

	if err := validateFolderList(next.Folder); err != nil {
		return false, err
	}

	// The fsnotify watcher is built once at startup; the new list needs a restart.
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

func (u *Unpackerr) putHooks(
	raw json.RawMessage, validate func([]*WebhookConfig) error, field func(*Config) *[]*WebhookConfig,
) error {
	var list []*WebhookConfig
	if err := unmarshalList(raw, &list); err != nil {
		return err
	}

	fileList := cloneHookList(list)

	if err := expandFilepaths(&list); err != nil {
		return err
	}

	if err := validate(list); err != nil {
		return err
	}

	return u.commitConfig(func(cfg *Config) {
		*field(cfg) = fileList
	}, func() {
		*field(u.Config) = list
		u.ensureHookWorker()
	})
}
