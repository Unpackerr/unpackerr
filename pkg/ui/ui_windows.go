//nolint:nosnakecase
package ui

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/gonutz/w32"
)

// SystrayIcon is the icon in the system tray or task bar.
const SystrayIcon = "files/windows.ico"

var hasGUI = os.Getenv("USEGUI") != "false" //nolint:gochecknoglobals

func HasGUI() bool {
	return hasGUI
}

// HideConsoleWindow hides the windows console window.
func HideConsoleWindow() {
	if console := w32.GetConsoleWindow(); console != 0 {
		_, consoleProcID := w32.GetWindowThreadProcessId(console)
		if w32.GetCurrentProcessId() == consoleProcID {
			w32.ShowWindowAsync(console, w32.SW_HIDE)
		}
	}
}

// ShowConsoleWindow does nothing on OSes besides Windows.
func ShowConsoleWindow() {
	if console := w32.GetConsoleWindow(); console != 0 {
		_, consoleProcID := w32.GetWindowThreadProcessId(console)
		if w32.GetCurrentProcessId() == consoleProcID {
			w32.ShowWindowAsync(console, w32.SW_SHOW)
		}
	}
}

// StartCmd starts a command.
func StartCmd(c string, v ...string) error {
	cmd := exec.Command(c, v...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	return cmd.Start() //nolint:wrapcheck
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
	return OpenCmd("PowerShell", "Get-Content", "-Tail", "1000", "-Wait", "-Encoding", "utf8", "-Path", "'"+logFile+"'")
}

// OpenFile open Config Files.
func OpenFile(filePath string) error {
	return OpenCmd("file://" + filePath)
}
