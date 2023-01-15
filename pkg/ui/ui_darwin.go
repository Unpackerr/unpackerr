package ui

import (
	"io"
	"os"
	"os/exec"
)

// SystrayIcon is the icon in the menu bar.
const SystrayIcon = "files/macos.png"

var hasGUI = os.Getenv("USEGUI") == "true" //nolint:gochecknoglobals

// HasGUI returns false on Linux, true on Windows and optional on macOS.
func HasGUI() bool {
	return hasGUI
}

// HideConsoleWindow does nothing on OSes besides Windows.
func HideConsoleWindow() {}

// ShowConsoleWindow does nothing on OSes besides Windows.
func ShowConsoleWindow() {}

// StartCmd starts a command.
func StartCmd(c string, v ...string) error {
	cmd := exec.Command(c, v...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	return cmd.Run()
}

// OpenCmd opens anything.
func OpenCmd(cmd ...string) error {
	return StartCmd("open", cmd...)
}

// OpenURL opens URL Links.
func OpenURL(url string) error {
	return OpenCmd(url)
}

// OpenLog opens Log Files.
func OpenLog(logFile string) error {
	return OpenCmd("-b", "com.apple.Console", logFile)
}

// OpenFile open Config Files.
func OpenFile(filePath string) error {
	return OpenCmd("-t", filePath)
}
