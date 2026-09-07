package unpackerr

import (
	"context"
	"time"
)

const restartShutdownTimeout = 5 * time.Second

// maybeRestart re-executes the process once a config PUT asked for a restart
// and nothing is mid-flight. Main loop only, from the cleaner tick.
func (u *Unpackerr) maybeRestart() {
	if !u.pendingRestart || !u.idle() {
		return
	}

	u.pendingRestart = false
	u.Printf("[Unpackerr] Config change needs a restart and the queue is idle; restarting now.")

	if u.Webserver != nil && u.Webserver.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), restartShutdownTimeout)
		defer cancel()

		_ = u.Webserver.server.Shutdown(ctx)
	}

	if err := restartProcess(); err != nil {
		u.Errorf("Restart failed, restart Unpackerr by hand to apply the saved config: %v", err)
	}
}

// idle reports whether a restart would interrupt an extraction or lose a
// pending delete. WAITING and EXTRACTFAILED items are rediscovered from the
// next Starr poll, so they do not block.
func (u *Unpackerr) idle() bool {
	if len(u.updates)+len(u.folders.Updates)+len(u.folders.Events)+len(u.hookChan)+len(u.delChan) > 0 {
		return false
	}

	for _, folder := range u.folders.Folders {
		if folder.status == QUEUED || folder.status == EXTRACTING {
			return false
		}
	}

	u.rLockHistory()
	defer u.rUnlockHistory()

	for _, item := range u.Map {
		switch item.Status {
		case QUEUED, EXTRACTING, EXTRACTED, IMPORTED, DELETING:
			return false
		case WAITING, EXTRACTFAILED, DELETED, DELETEFAILED, EXTRACTEDNOTHING:
		}
	}

	return true
}
