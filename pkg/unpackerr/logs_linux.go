//go:build linux

package unpackerr

/* The purpose of this code is to log stderr (application panics) to a log file. */

import (
	"math"
	"os"

	"golang.org/x/sys/unix"
)

func redirectStderr(file *os.File) {
	os.Stderr = file

	if fd := file.Fd(); fd <= uintptr(math.MaxInt) {
		_ = unix.Dup3(int(fd), unix.Stderr, 0)
	}
}
