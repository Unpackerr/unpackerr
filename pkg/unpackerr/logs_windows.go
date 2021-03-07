package unpackerr

import (
	"syscall"
)

// From https://play.golang.org/p/ue8ULfyHGG.
func dupFD2(oldfd uintptr, newfd uintptr) error {
	r0, _, e1 := syscall.Syscall(syscall.MustLoadDLL("kernel32.dll").
		MustFindProc("SetStdHandle").Addr(), 2, oldfd, newfd, 0)
	if r0 == 0 {
		if e1 != 0 {
			return error(e1)
		}

		return syscall.EINVAL
	}

	return nil
}
