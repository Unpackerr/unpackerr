package unpackerr

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/davidnewhall/unpackerr/pkg/bindata"
	"github.com/davidnewhall/unpackerr/pkg/ui"
	homedir "github.com/mitchellh/go-homedir"
	"golift.io/cnfg"
	"golift.io/cnfgfile"
)

const (
	msgNoConfigFile = "Using env variables only. Config file not found."
	msgConfigFailed = "Using env variables only. Could not create config file: "
	msgConfigCreate = "Created new config file: "
	msgConfigFound  = "Using Config File: "
)

func (u *Unpackerr) unmarshalConfig() (uint64, uint64, string, error) {
	var f, msg string

	def, cfl := configFileLocactions()
	for _, f = range append([]string{u.Flags.ConfigFile}, cfl...) {
		d, err := homedir.Expand(f)
		if err == nil {
			f = d
		}

		if _, err := os.Stat(f); err == nil {
			break
		} // else { u.Print("rip:", err) }

		f = ""
	}

	msg = msgNoConfigFile

	if f != "" {
		u.Flags.ConfigFile, _ = filepath.Abs(f)
		msg = msgConfigFound + u.Flags.ConfigFile

		if err := cnfgfile.Unmarshal(u.Config, u.Flags.ConfigFile); err != nil {
			return 0, 0, msg, fmt.Errorf("config file: %w", err)
		}
	} else if f, err := u.createConfigFile(def); err != nil {
		msg = msgConfigFailed + err.Error()
	} else if f != "" {
		u.Flags.ConfigFile = f
		msg = msgConfigCreate + u.Flags.ConfigFile
	}

	if _, err := cnfg.UnmarshalENV(u.Config, u.Flags.EnvPrefix); err != nil {
		return 0, 0, msg, fmt.Errorf("environment variables: %w", err)
	}

	fm, dm := u.validateConfig()

	return fm, dm, msg, nil
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

	fm, err := strconv.ParseUint(u.FileMode, bits, base)
	if err != nil || u.FileMode == "" {
		fm = defaultFileMode
		u.FileMode = strconv.FormatUint(fm, bits)
	}

	dm, err := strconv.ParseUint(u.DirMode, bits, base)
	if err != nil || u.DirMode == "" {
		dm = defaultDirMode
		u.DirMode = strconv.FormatUint(dm, bits)
	}

	if u.Parallel == 0 {
		u.Parallel++
	}

	if u.Buffer == 0 {
		u.Buffer = defaultFolderBuf
	} else if u.Buffer < minimumFolderBuf {
		u.Buffer = minimumFolderBuf
	}

	if u.Interval.Duration < minimumInterval {
		u.Interval.Duration = minimumInterval
	}

	if u.Config.Debug && u.LogFiles == defaultLogFiles {
		u.LogFiles *= 2 // Double default if debug is turned on.
	}

	if u.LogFileMb == 0 {
		if u.LogFileMb = defaultLogFileMb; u.Config.Debug {
			u.LogFileMb *= 2 // Double default if debug is turned on.
		}
	}

	if ui.HasGUI() && u.LogFile == "" {
		u.LogFile = filepath.Join("~", ".unpackerr", "unpackerr.log")
	}

	if u.KeepHistory != 0 {
		u.History.Items = make([]string, u.KeepHistory)
	}

	return fm, dm
}

func (u *Unpackerr) createConfigFile(file string) (string, error) {
	if !ui.HasGUI() {
		return "", nil
	}

	file, err := homedir.Expand(file)
	if err != nil {
		return "", fmt.Errorf("expanding home: %w", err)
	}

	if file, err = filepath.Abs(file); err != nil {
		return "", fmt.Errorf("absolute file: %w", err)
	}

	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, logsDirMode); err != nil {
		return "", fmt.Errorf("making config dir: %w", err)
	}

	f, err := os.Create(file)
	if err != nil {
		return "", fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	if a, err := bindata.Asset("../../examples/unpackerr.conf.example"); err != nil {
		return "", fmt.Errorf("getting config file: %w", err)
	} else if _, err = f.Write(a); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	if err := cnfgfile.Unmarshal(u.Config, file); err != nil {
		return file, fmt.Errorf("config file: %w", err)
	}

	return file, nil
}
