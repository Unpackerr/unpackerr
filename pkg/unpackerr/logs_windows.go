//go:build windows

package unpackerr

/* The purpose of this code is to log stderr (application panics) to a log file. */

import (
	"os"

	winsys "golang.org/x/sys/windows"
)

func redirectStderr(file *os.File) {
	os.Stderr = file
	// Use the typed Windows API; Handle is the correct type for Fd on Windows (no uintptr/Syscall).
	_ = winsys.SetStdHandle(winsys.STD_ERROR_HANDLE, winsys.Handle(file.Fd()))
}
