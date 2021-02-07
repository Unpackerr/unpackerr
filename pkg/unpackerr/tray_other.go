// +build !windows,!darwin

package unpackerr

import (
	"os"
	"os/signal"
	"syscall"
)

func (u *Unpackerr) startTray() {
	signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, <-u.sigChan)
}
