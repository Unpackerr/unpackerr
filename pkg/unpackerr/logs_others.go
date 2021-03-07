//+build !linux,!windows

package unpackerr

import (
	"syscall"
)

func dupFD2(oldfd uintptr, newfd uintptr) error {
	return syscall.Dup2(int(oldfd), int(newfd))
}
