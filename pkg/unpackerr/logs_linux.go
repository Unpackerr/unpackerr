package unpackerr

import (
	"os"
	"syscall"
)

func dupStderr(file *os.File) {
	os.Stderr = file
	_ = syscall.Dup3(int(file.Fd()), syscall.Stderr, 0)
}
