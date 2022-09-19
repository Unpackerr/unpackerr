package unpackerr

/* The purpose of this code is to log stderr (application panics) to a log file. */

import (
	"os"
	"syscall"
)

//nolint:gochecknoglobals // These can be reused if needed.
var (
	kernel    = syscall.MustLoadDLL("kernel32.dll")
	setHandle = kernel.MustFindProc("SetStdHandle")
)

//nolint:errcheck
func redirectStderr(file *os.File) {
	os.Stderr = file
	stderr := syscall.STD_ERROR_HANDLE //nolint:nosnakecase

	const noIdeaWhatThisIs = 2
	//nolint:staticcheck // I have no idea how to do this with syscallN. :( but we need to...
	syscall.Syscall(setHandle.Addr(), noIdeaWhatThisIs, uintptr(stderr), file.Fd(), 0)
}
