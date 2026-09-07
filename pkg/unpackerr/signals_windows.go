//go:build windows

package unpackerr

import (
	"os"
	"os/signal"
	"syscall"
)

func notifySignals(ch chan os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
}

func isHangup(_ os.Signal) bool {
	return false
}
