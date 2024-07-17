package unpackerr

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Unpackerr/unpackerr/examples"
	"github.com/Unpackerr/unpackerr/pkg/ui"
	"github.com/hako/durafmt"
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
)

func (u *Unpackerr) unmarshalConfig() (uint64, uint64, string, error) {
	var configFile, msg string

	// Load up the default file path and a list of alternate paths.
	def, cfl := configFileLocactions()
	// Search for one, starting with the default.
	for _, configFile = range append([]string{u.Flags.ConfigFile}, cfl...) {
		configFile = expandHomedir(configFile)
		if _, err := os.Stat(configFile); err == nil {
			break // found one, bail out.
		} // else { u.Print("rip:", err) }

		configFile = ""
	}

	// it's possible to get here with or without a file found.
	msg = msgNoConfigFile

	if configFile != "" {
		u.Flags.ConfigFile, _ = filepath.Abs(configFile)
		msg = msgConfigFound + u.Flags.ConfigFileWithAge()

		if err := cnfgfile.Unmarshal(u.Config, u.Flags.ConfigFile); err != nil {
			return 0, 0, msg, fmt.Errorf("config file: %w", err)
		}
	} else if f, err := u.createConfigFile(def); err != nil {
		msg = msgConfigFailed + err.Error()
	} else if f != "" {
		u.Flags.ConfigFile = f
		msg = msgConfigCreate + u.Flags.ConfigFileWithAge()
	}

	if _, err := cnfg.UnmarshalENV(u.Config, u.Flags.EnvPrefix); err != nil {
		return 0, 0, msg, fmt.Errorf("environment variables: %w", err)
	}

	if err := u.setPasswords(); err != nil {
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

	age := durafmt.Parse(time.Since(stat.ModTime())).LimitFirstN(3) //nolint:mnd

	return f.ConfigFile + ", age: " + age.Format(durafmtUnits)
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
func (u *Unpackerr) validateConfig() (uint64, uint64) { //nolint:cyclop
	if u.DeleteDelay.Duration > 0 && u.DeleteDelay.Duration < minimumDeleteDelay {
		u.DeleteDelay.Duration = minimumDeleteDelay
	}

	const (
		bits = 8
		base = 32
	)

	fileMode, err := strconv.ParseUint(u.FileMode, bits, base)
	if err != nil || u.FileMode == "" {
		fileMode = defaultFileMode
		u.FileMode = strconv.FormatUint(fileMode, bits)
	}

	dirMode, err := strconv.ParseUint(u.DirMode, bits, base)
	if err != nil || u.DirMode == "" {
		dirMode = defaultDirMode
		u.DirMode = strconv.FormatUint(dirMode, bits)
	}

	if u.Parallel == 0 {
		u.Parallel++
	}

	if u.Folder.Buffer == 0 {
		u.Folder.Buffer = defaultFolderBuf
	} else if u.Folder.Buffer < minimumFolderBuf {
		u.Folder.Buffer = minimumFolderBuf
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
	}

	if u.StartDelay.Duration < minimumInterval {
		u.StartDelay.Duration = minimumInterval
	}

	if u.LogQueues.Duration < minimumInterval {
		u.LogQueues.Duration = minimumInterval
	}

	if u.ErrorStdErr && runtime.GOOS == windows {
		u.ErrorStdErr = false // no stderr on windows
	}

	if ui.HasGUI() && u.LogFile == "" {
		u.LogFile = filepath.Join("~", ".unpackerr", "unpackerr.log")
	}

	if u.KeepHistory != 0 {
		u.History.Items = make([]string, u.KeepHistory)
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

	fOpen, err := os.Create(file)
	if err != nil {
		return "", fmt.Errorf("creating config file: %w", err)
	}
	defer fOpen.Close()

	if _, err = fOpen.Write(examples.ConfigFile); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	if err := cnfgfile.Unmarshal(u.Config, file); err != nil {
		return file, fmt.Errorf("config file: %w", err)
	}

	return file, nil
}

// This function checks if rar passwords need to be read from a file path.
// Only runs once at startup to load passwords into memory.
func (u *Unpackerr) setPasswords() error {
	const filePrefix = "filepath:"

	newPasswords := []string{}

	for _, pass := range u.Passwords {
		if !strings.HasPrefix(pass, filePrefix) {
			newPasswords = append(newPasswords, pass)
			continue
		}

		fileContent, err := os.ReadFile(strings.TrimPrefix(pass, filePrefix))
		if err != nil {
			return fmt.Errorf("reading password file: %w", err)
		}

		filePasswords := strings.Split(string(fileContent), "\n")
		if len(filePasswords) > 0 && filePasswords[len(filePasswords)-1] == "" {
			// Remove the last "password" if it's blank (newline at end of file).
			filePasswords = filePasswords[:len(filePasswords)-1]
		}

		newPasswords = append(newPasswords, filePasswords...)
	}

	u.Passwords = newPasswords

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
	if conf.URL == "" {
		u.Errorf("Missing %s URL in one of your configurations, skipped and ignored.", app)
		return ErrInvalidURL // this error is not printed.
	}

	if conf.APIKey == "" {
		u.Errorf("Missing %s API Key in one of your configurations, skipped and ignored.", app)
		return ErrInvalidURL // this error is not printed.
	}

	if !strings.HasPrefix(conf.URL, "http://") && !strings.HasPrefix(conf.URL, "https://") {
		return fmt.Errorf("%w: (%s) %s", ErrInvalidURL, app, conf.URL)
	}

	if len(conf.APIKey) != apiKeyLength {
		return fmt.Errorf("%s (%s) %w, your key length: %d",
			app, conf.URL, ErrInvalidKey, len(conf.APIKey))
	}

	if conf.Timeout.Duration == 0 {
		conf.Timeout.Duration = u.Timeout.Duration
	}

	if conf.DeleteDelay.Duration == 0 {
		conf.DeleteDelay.Duration = u.DeleteDelay.Duration
	}

	if conf.Path != "" {
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

	conf.Config.Client = &http.Client{
		Timeout: conf.Timeout.Duration,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !conf.ValidSSL}, //nolint:gosec
		},
	}

	return nil
}
