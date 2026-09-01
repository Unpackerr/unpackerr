//go:build !windows && !darwin

package unpackerr

func (u *Unpackerr) startTray() {
	go u.Run()

	defer u.Stop() // stop and wait for extractions to finish.

	u.waitForExit()
}

func (u *Unpackerr) updateTray(_ *Stats, _ uint) {
	// there is no tray.
}
