//go:build !windows && !darwin

package ui

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// SystrayIcon is the icon in the system tray or task bar.
const SystrayIcon = "files/favicon.png"

// HasGUI returns false on Linux, true on Windows and optional on macOS.
func HasGUI() bool {
	return false
}

// StartCmd starts a command.
func StartCmd(c string, v ...string) error {
	cmd := exec.Command(c, v...) //nolint:noctx // we should fix this.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	return cmd.Start() //nolint:wrapcheck
}

// ErrUnsupported is just an error.
var ErrUnsupported = errors.New("unsupported OS")

// OpenCmd opens anything.
func OpenCmd(_ ...string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenURL opens URL Links.
func OpenURL(_ string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenLog opens Log Files.
func OpenLog(_ string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}

// OpenFile open Config Files.
func OpenFile(_ string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, runtime.GOOS)
}
