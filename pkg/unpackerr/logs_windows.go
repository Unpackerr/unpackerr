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
	h := syscall.STD_ERROR_HANDLE
	f := file.Fd()

	syscall.Syscall(setHandle.Addr(), 2, uintptr(h), uintptr(f), 0)
}
