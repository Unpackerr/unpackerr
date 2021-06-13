//+build !linux,!windows

package unpackerr

import (
	"os"
	"syscall"
)

func dupStderr(file *os.File) {
	os.Stderr = file
	_ = syscall.Dup2(int(file.Fd()), syscall.Stderr)
}
