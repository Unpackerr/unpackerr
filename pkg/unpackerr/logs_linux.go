package unpackerr

import (
	"syscall"
)

func dupFD2(oldfd uintptr, newfd uintptr) error {
	return syscall.Dup3(int(oldfd), int(newfd), 0) //nolint:wrapcheck
}
