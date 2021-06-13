package unpackerr

import (
	"os"
	"syscall"
)

// nolint:gochecknoglobals // These can be reused if needed.
var (
	kernel    = syscall.MustLoadDLL("kernel32.dll")
	setHandle = kernel.MustFindProc("SetStdHandle")
)

//nolint:errcheck
func dupStderr(file *os.File) {
	os.Stderr = file
	syscall.Syscall(setHandle.Addr(), 2, file.Fd(), uintptr(syscall.Stderr), 0)
}
