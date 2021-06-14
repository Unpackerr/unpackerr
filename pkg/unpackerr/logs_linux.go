package unpackerr

/* The purpose of this code is to log stderr (application panics) to a log file. */

import (
	"os"
	"syscall"
)

func redirectStderr(file *os.File) {
	os.Stderr = file
	_ = syscall.Dup3(int(file.Fd()), syscall.Stderr, 0)
}
