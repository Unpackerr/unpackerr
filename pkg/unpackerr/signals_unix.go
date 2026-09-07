//go:build !windows

package unpackerr

import (
	"os"
	"os/signal"
	"syscall"
)

func notifySignals(ch chan os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)
}

func isHangup(sig os.Signal) bool {
	return sig == syscall.SIGHUP
}
