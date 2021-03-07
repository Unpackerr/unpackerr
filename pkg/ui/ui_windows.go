package ui

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// SystrayIcon is the icon in the system tray or task bar.
const SystrayIcon = "files/windows.ico"

var hasGUI = os.Getenv("USEGUI") != "false" // nolint:gochecknoglobals

func HasGUI() bool {
	return hasGUI
}

// StartCmd starts a command.
func StartCmd(c string, v ...string) error {
	cmd := exec.Command(c, v...)
	cmd.Stdout = ioutil.Discard
	cmd.Stderr = ioutil.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	return cmd.Start()
}

// OpenCmd opens anything.
func OpenCmd(cmd ...string) error {
	return StartCmd("cmd", append([]string{"/c", "start"}, cmd...)...)
}

// OpenURL opens URL Links.
func OpenURL(url string) error {
	return OpenCmd(strings.ReplaceAll(url, "&", "^&"))
}

// OpenLog opens Log Files.
func OpenLog(logFile string) error {
	return OpenCmd("PowerShell", "Get-Content", "-Tail", "1000", "-Wait", "-Encoding", "utf8", "-Path", logFile)
}

// OpenFile open Config Files.
func OpenFile(filePath string) error {
	return OpenCmd("file://" + filePath)
}
