//+build !linux,!windows

package unpackerr

/* The purpose of this code is to log stderr (application panics) to a log file. */

import (
	"os"
	"syscall"
)

func redirectStderr(file *os.File) {
	os.Stderr = file
	// This works on darwin and freebsd, maybe others.
	_ = syscall.Dup2(int(file.Fd()), syscall.Stderr)
}
