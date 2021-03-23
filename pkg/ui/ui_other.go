// +build !windows,!darwin

package ui

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
)

// SystrayIcon is the icon in the system tray or task bar.
const SystrayIcon = "files/favicon.png"

// HasGUI returns false on Linux, true on Windows and optional on macOS.
func HasGUI() bool {
	return false
}

// HideConsoleWindow does nothing on OSes besides Windows.
func HideConsoleWindow() {}

// ShowConsoleWindow does nothing on OSes besides Windows.
func ShowConsoleWindow() {}

// StartCmd starts a command.
func StartCmd(c string, v ...string) error {
	cmd := exec.Command(c, v...)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = ioutil.Discard

	return cmd.Start() //nolint:wrapcheck
}

// ErrUnsupported is just an error.
var ErrUnsupported = fmt.Errorf("unsupported OS")

// OpenCmd opens anything.
func OpenCmd(cmd ...string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenURL opens URL Links.
func OpenURL(url string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenLog opens Log Files.
func OpenLog(logFile string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenFile open Config Files.
func OpenFile(filePath string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}
