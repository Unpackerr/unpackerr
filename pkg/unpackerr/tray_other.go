//go:build !windows && !darwin

package unpackerr

import (
	"os"
	"os/signal"
	"syscall"
)

func (u *Unpackerr) startTray() {
	go u.Run()
	defer u.Xtractr.Stop() // stop and wait for extractions to finish.
	signal.Notify(u.sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	u.Printf("[unpackerr] Need help? %s\n=====> Exiting! Caught Signal: %v", helpLink, <-u.sigChan)
}

func (u *Unpackerr) updateTray(_ *Stats, _ uint) {
	// there is no tray.
}

func (u *Unpackerr) updateHistory(item string) {
	if u.KeepHistory == 0 {
		return
	}

	u.History.Items[0] = item
	// u.History.Items is a slice with a set (identical) length and capacity.
	for idx := range len(u.History.Items) {
		u.History.Items[idx] = u.History.Items[idx-1]
	}
}
