package unpackerr

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/configdef"
	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/dromara/carbon/v2"
	homedir "github.com/mitchellh/go-homedir"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
	"golift.io/starr"
)

const (
	msgNoConfigFile = "Using env variables only. Config file not found."
	msgConfigFailed = "Using env variables only. Could not create config file: "
	msgConfigCreate = "Created new config file: "
	msgConfigFound  = "Using Config File: "
	filePrefix      = "filepath:"
)

var (
	errNoConfigFile   = errors.New("no config file path")
	errNoFileSnapshot = errors.New("no on-disk config snapshot")
)

func (u *Unpackerr) unmarshalConfig() (uint64, uint64, string, error) {
	var configFile, msg string

	// Load up the default file path and a list of alternate paths.
	def, cfl := configFileLocactions()
	// Search for one, starting with the default.
	for _, configFile = range append([]string{u.ConfigFile}, cfl...) {
		configFile = expandHomedir(configFile)
		if _, err := os.Stat(configFile); err == nil {
			break // found one, bail out.
		} // else { u.Print("rip:", err) }

		configFile = ""
	}

	// it's possible to get here with or without a file found.
	msg = msgNoConfigFile

	if configFile != "" {
		u.ConfigFile, _ = filepath.Abs(configFile)
		msg = msgConfigFound + u.ConfigFileWithAge()

		if err := cnfgfile.Unmarshal(u.Config, u.ConfigFile); err != nil {
			return 0, 0, msg, fmt.Errorf("config file: %w", err)
		}
	} else if f, err := u.createConfigFile(def); err != nil {
		msg = msgConfigFailed + err.Error()
	} else if f != "" {
		u.ConfigFile = f
		msg = msgConfigCreate + u.ConfigFileWithAge()
	}

	// File snapshot first so UN_* overlays stay on the live Config and never get written back.
	u.snapshotFileConfig()

	res, err := cnfg.ParseENV(u.Config, u.EnvPrefix)
	if err != nil {
		return 0, 0, msg, fmt.Errorf("environment variables: %w", err)
	}

	u.envUsed = envSuffixes(res.Used, u.EnvPrefix)

	u.snapshotLivePasswords()

	if err := u.setPasswords(); err != nil {
		return 0, 0, msg, err
	}

	if err := u.setupUIPassword(); err != nil {
		return 0, 0, msg, err
	}

	if err := u.Webserver.validateAuth(); err != nil {
		return 0, 0, msg, err
	}

	u.Webserver.normalizeURLBase()

	if err := u.Webserver.validateURLBase(); err != nil {
		return 0, 0, msg, err
	}

	fileMode, dirMode := u.validateConfig()

	return fileMode, dirMode, msg, nil
}

func (f *Flags) ConfigFileWithAge() string {
	stat, err := os.Stat(f.ConfigFile)
	if err != nil {
		return f.ConfigFile + ", unknown age"
	}

	age := carbon.CreateFromStdTime(stat.ModTime()).DiffAbsInString()

	return f.ConfigFile + ", age: " + age
}

func configFileLocactions() (string, []string) {
	switch runtime.GOOS {
	case windows:
		return `C:\ProgramData\unpackerr\unpackerr.conf`, []string{
			`~\.unpackerr\unpackerr.conf`,
			`C:\ProgramData\unpackerr\unpackerr.conf`,
			`.\unpackerr.conf`,
		}
	case "darwin":
		return "~/.unpackerr/unpackerr.conf", []string{
			"/usr/local/etc/unpackerr/unpackerr.conf",
			"/etc/unpackerr/unpackerr.conf",
			"~/.unpackerr/unpackerr.conf",
			"./unpackerr.conf",
		}
	case "freebsd", "netbsd", "openbsd":
		return "", []string{
			"/usr/local/etc/unpackerr/unpackerr.conf",
			"/etc/unpackerr/unpackerr.conf",
			"~/.unpackerr/unpackerr.conf",
			"./unpackerr.conf",
		}
	case "android", "dragonfly", "linux", "nacl", "plan9", "solaris":
		fallthrough
	default:
		// Adding a default here, or to freebsd changes the behavior of createConfigFile, so don't.
		return "", []string{
			"/etc/unpackerr/unpackerr.conf",
			"/config/unpackerr.conf",
			"/usr/local/etc/unpackerr/unpackerr.conf",
			"~/.unpackerr/unpackerr.conf",
			"./unpackerr.conf",
		}
	}
}

// validateConfig makes sure config file values are ok. Returns file and dir modes.
func (u *Unpackerr) validateConfig() (uint64, uint64) {
	u.ensureTrayRing()

	return clampConfig(u.Config)
}

// ensureTrayRing sizes the GUI history ring. This is tray-only; the API and web
// UI read /api/history instead, so it goes away with the tray history menu.
func (u *Unpackerr) ensureTrayRing() {
	if u.KeepHistory != 0 && len(u.Items) == 0 {
		u.Items = make([]string, min(u.KeepHistory, trayHistory))
	}
}

// clampConfig applies minimums and fills defaults for omitted values. It takes a
// *Config so a config PUT can clamp a staged copy and compare that against live,
// instead of comparing raw input against already-clamped values.
func clampConfig(cfg *Config) (uint64, uint64) { //nolint:cyclop
	if cfg.DeleteDelay.Duration > 0 && cfg.DeleteDelay.Duration < minimumDeleteDelay {
		cfg.DeleteDelay.Duration = minimumDeleteDelay
	}

	if _, err := strconv.ParseUint(cfg.LogFileMode, bits8, base32); err != nil || cfg.LogFileMode == "" {
		cfg.LogFileMode = strconv.FormatUint(defaultLogFileMode, bits8)
	}

	fileMode, err := strconv.ParseUint(cfg.FileMode, bits8, base32)
	if err != nil || cfg.FileMode == "" {
		fileMode = defaultFileMode
		cfg.FileMode = strconv.FormatUint(fileMode, bits8)
	}

	dirMode, err := strconv.ParseUint(cfg.DirMode, bits8, base32)
	if err != nil || cfg.DirMode == "" {
		dirMode = defaultDirMode
		cfg.DirMode = strconv.FormatUint(dirMode, bits8)
	}

	if cfg.Parallel == 0 {
		cfg.Parallel++
	}

	if cfg.Progress.Duration == 0 {
		cfg.Progress.Duration = defaultProgressInterval
	} else if cfg.Progress.Duration < minimumProgressInterval {
		cfg.Progress.Duration = minimumProgressInterval
	}

	if cfg.Folder.Buffer == 0 {
		cfg.Folder.Buffer = defaultFolderBuf
	} else if cfg.Folder.Buffer < minimumFolderBuf {
		cfg.Folder.Buffer = minimumFolderBuf
	}

	if cfg.Interval.Duration < minimumInterval {
		cfg.Interval.Duration = minimumInterval
	}

	if cfg.StartDelay.Duration < minimumInterval {
		cfg.StartDelay.Duration = minimumInterval
	}

	if cfg.LogQueues.Duration < minimumInterval {
		cfg.LogQueues.Duration = minimumInterval
	}

	if cfg.ErrorStdErr && runtime.GOOS == windows {
		cfg.ErrorStdErr = false // no stderr on windows
	}

	if ui.HasGUI() && cfg.LogFile == "" {
		cfg.LogFile = filepath.Join("~", ".unpackerr", "unpackerr.log")
	}

	return fileMode, dirMode
}

// createConfigFile attempts to avoid creating a config file on linux or freebsd.
// It used to avoid it when running on macos from homebrew, but not anymore.
func (u *Unpackerr) createConfigFile(file string) (string, error) {
	if isRunningInDocker() {
		if stat, err := os.Stat("/config"); err == nil && stat.IsDir() {
			file = "/config/unpackerr.conf"
		}
	}

	if file == "" {
		return "", nil
	}

	file, err := filepath.Abs(expandHomedir(file))
	if err != nil {
		return "", fmt.Errorf("absolute file: %w", err)
	}

	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, logsDirMode); err != nil {
		return "", fmt.Errorf("making config dir: %w", err)
	}

	schema, err := configdef.Load()
	if err != nil {
		return "", fmt.Errorf("definitions: %w", err)
	}

	if err := configdef.AtomicWrite(file, []byte(schema.ExampleTOML())); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	if err := cnfgfile.Unmarshal(u.Config, file); err != nil {
		return file, fmt.Errorf("config file: %w", err)
	}

	return file, nil
}

// writeConfigFile atomically rewrites the active config file from the on-disk snapshot.
func (u *Unpackerr) writeConfigFile() error {
	u.configMu.Lock()
	defer u.configMu.Unlock()

	return u.writeConfigFrom(u.fileConfig)
}

func (u *Unpackerr) writeConfigFrom(cfg *Config) error {
	if strings.TrimSpace(u.ConfigFile) == "" {
		return errNoConfigFile
	}

	schema, err := configdef.Load()
	if err != nil {
		return fmt.Errorf("%w: %w", errPersistConfig, err)
	}

	if cfg == nil {
		return errNoFileSnapshot
	}

	body := schema.RenderTOML(cfg, configdef.RenderOpts{Mode: configdef.RenderLive})

	if err := configdef.AtomicWrite(u.ConfigFile, []byte(body)); err != nil {
		return fmt.Errorf("%w: %w", errPersistConfig, err)
	}

	return nil
}

// persistConfigFile writes the on-disk snapshot. Failure is recorded, not returned,
// so a read-only config (puppet, container) still starts with in-memory values.
func (u *Unpackerr) persistConfigFile() {
	err := u.writeConfigFile()
	switch {
	case err == nil:
		u.configWriteErr = nil
	case errors.Is(err, errNoConfigFile):
		return
	default:
		u.configWriteErr = err
	}
}

func (u *Unpackerr) snapshotFileConfig() {
	u.fileConfig = cloneConfig(u.Config)
}

// envSuffixes strips the parser prefix so the UI matches envVar="DEBUG" against UN_DEBUG.
// cnfg joins prefix + "_" + tag, so a prefix that already ends in "_" (APP_)
// produces APP__DEBUG; we strip that exact prefix and keep the rest as stored
// (map keys like WEBSERVER_ROLES_stats_PERMISSIONS_0 stay mixed-case).
func envSuffixes(used cnfg.Pairs, prefix string) map[string]string {
	out := make(map[string]string, len(used))
	pfx := prefix + cnfg.LevelSeparator

	for key, val := range used {
		name := key
		if prefix != "" {
			if cut, ok := strings.CutPrefix(key, pfx); ok {
				name = cut
			}
		}

		out[name] = val
	}

	return out
}

func envValueSecret(suffix string) bool {
	name := strings.ToUpper(suffix)
	if strings.Contains(name, "PASSWORD") || strings.Contains(name, "_PASS") ||
		name == "API_KEY" || strings.HasSuffix(name, "_API_KEY") ||
		strings.HasSuffix(name, "_TOKEN") {
		return true
	}

	_, afterKeys, found := strings.Cut(name, "API_KEYS_")
	if !found {
		return false
	}

	_, after, ok := strings.Cut(afterKeys, "_")

	return ok && after == "KEY"
}

func (u *Unpackerr) syncFileUIPassword() {
	pass := u.uiPassword()

	u.configMu.Lock()
	defer u.configMu.Unlock()

	if u.fileConfig == nil {
		return
	}

	if u.fileConfig.Webserver == nil {
		u.fileConfig.Webserver = &WebServer{}
	}

	u.fileConfig.Webserver.UIPassword = pass
}

func (u *Unpackerr) appendFileAPIKey(key APIKey) {
	u.configMu.Lock()
	defer u.configMu.Unlock()

	if u.fileConfig == nil {
		return
	}

	if u.fileConfig.Webserver == nil {
		u.fileConfig.Webserver = &WebServer{}
	}

	cloned := cloneAPIKeys([]APIKey{key})
	u.fileConfig.Webserver.APIKeys = append(u.fileConfig.Webserver.APIKeys, cloned...)
}

func (u *Unpackerr) snapshotLivePasswords() {
	u.livePasswords = make(StringSlice, len(u.Passwords))
	copy(u.livePasswords, u.Passwords)
}

func expandPasswords(passwords StringSlice) (StringSlice, error) {
	newPasswords := []string{}

	for _, pass := range passwords {
		if !strings.HasPrefix(pass, filePrefix) {
			newPasswords = append(newPasswords, pass)
			continue
		}

		fileContent, err := os.ReadFile(strings.TrimPrefix(pass, filePrefix))
		if err != nil {
			return nil, fmt.Errorf("reading password file: %w", err)
		}

		filePasswords := strings.Split(string(fileContent), "\n")
		if len(filePasswords) > 0 && filePasswords[len(filePasswords)-1] == "" {
			// Remove the last "password" if it's blank (newline at end of file).
			filePasswords = filePasswords[:len(filePasswords)-1]
		}

		newPasswords = append(newPasswords, filePasswords...)
	}

	return newPasswords, nil
}

// This function checks if rar passwords need to be read from a file path.
// Only runs once at startup to load passwords into memory.
func (u *Unpackerr) setPasswords() error {
	u.snapshotLivePasswords()

	expanded, err := expandPasswords(u.Passwords)
	if err != nil {
		return err
	}

	u.Passwords = expanded

	return nil
}

// only run this once.
func isRunningInDocker() bool {
	// docker creates a .dockerenv file at the root of the container.
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// expandHomedir expands a ~ to a homedir, or returns the original path in case of any error.
func expandHomedir(filePath string) string {
	expanded, err := homedir.Expand(filePath)
	if err != nil {
		return filePath
	}

	return expanded
}

func (u *Unpackerr) validateApp(conf *StarrConfig, app starr.App) error {
	conf.Name = strings.TrimSpace(conf.Name)

	if conf.URL == "" {
		u.Errorf("Missing %s URL in one of your configurations, skipped and ignored.", app)
		return ErrInvalidURL // this error is not printed.
	}

	if conf.APIKey == "" {
		u.Errorf("Missing %s API Key in one of your configurations, skipped and ignored.", app)
		return ErrInvalidKey // this error is not printed at startup; PUT returns it.
	}

	if !strings.HasPrefix(conf.URL, "http://") && !strings.HasPrefix(conf.URL, "https://") {
		return fmt.Errorf("%w: (%s) %s", ErrInvalidURL, app, conf.URL)
	}

	if len(conf.APIKey) < apiKeyMinLength {
		u.Errorf("%s (%s) API Key is too short (%d < %d), skipped and ignored.",
			app, conf.URL, len(conf.APIKey), apiKeyMinLength)

		return fmt.Errorf("%s (%s) %w, your key length: %d",
			app, conf.URL, ErrInvalidKey, len(conf.APIKey))
	}

	if conf.Timeout.Duration == 0 {
		conf.Timeout.Duration = u.Timeout.Duration
	}

	if conf.DeleteDelay.Duration == 0 {
		conf.DeleteDelay.Duration = u.DeleteDelay.Duration
	}

	if conf.Path != "" && !slices.Contains(conf.Paths, conf.Path) {
		conf.Paths = append(conf.Paths, conf.Path)
	}

	for idx, path := range conf.Paths {
		conf.Paths[idx] = expandHomedir(path)
	}

	if len(conf.Paths) == 0 {
		conf.Paths = []string{defaultSavePath}
	}

	if conf.Protocols == "" {
		conf.Protocols = defaultProtocol
	}

	if err := conf.applyMaxBytes(app); err != nil {
		return fmt.Errorf("%s (%s) %w", app, conf.URL, err)
	}

	conf.Client = &http.Client{
		Timeout: conf.Timeout.Duration,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !conf.ValidSSL}, //nolint:gosec
		},
	}

	return nil
}

func defaultAppMaxBytes(app starr.App) string {
	switch app {
	case starr.Sonarr:
		return defaultSonarrMaxBytes
	case starr.Radarr:
		return defaultRadarrMaxBytes
	case starr.Lidarr:
		return defaultLidarrMaxBytes
	case starr.Readarr:
		return defaultReadarrMaxBytes
	case starr.Whisparr:
		return defaultWhisparrMaxBytes
	default:
		return defaultSonarrMaxBytes
	}
}

func (conf *StarrConfig) applyMaxBytes(app starr.App) error {
	size := strings.TrimSpace(conf.MaxBytes)
	if size == "" {
		size = defaultAppMaxBytes(app)
	}

	n, err := parseExtractMaxBytes(size)
	if err != nil {
		return err
	}

	conf.maxBytes = n

	return nil
}
