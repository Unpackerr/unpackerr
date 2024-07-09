//go:build !windows

package unpackerr

import "syscall"

const defaultSavePath = "/downloads"

func getUmask() int {
	umask := syscall.Umask(0)
	syscall.Umask(umask)

	return umask
}
