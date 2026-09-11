package unpackerr

import (
	"context"
	"os"
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

	// Resolve the binary before anything is torn down. Past this point the
	// listener is gone, so the only fallible step has to happen first.
	exe, err := os.Executable()
	if err != nil {
		u.Errorf("Config saved, but the Unpackerr binary path is unknown, "+
			"so it needs a manual restart to apply: %v", err)

		return
	}

	u.Printf("[Unpackerr] Config change needs a restart and the queue is idle; restarting now.")

	if u.Webserver != nil && u.Webserver.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), restartShutdownTimeout)
		_ = u.Webserver.server.Shutdown(ctx)

		cancel()
	}

	if err := restartProcess(exe); err != nil {
		u.Errorf("Restart failed; the saved config needs a manual restart to apply: %v", err)
		return
	}

	os.Exit(0) // Windows only: the replacement is already running. exec(2) never returns here.
}

// idle reports whether a restart would interrupt an extraction or lose a
// pending delete. WAITING and EXTRACTFAILED items are rediscovered from the
// next Starr poll, so they do not block.
func (u *Unpackerr) idle() bool {
	// inFlight covers deletes and hooks from the send until the worker is
	// done, so their channel depth is already accounted for.
	if u.inFlight.Load() > 0 {
		return false
	}

	if len(u.updates)+len(u.folders.Updates)+len(u.folders.Events) > 0 {
		return false
	}

	for _, folder := range u.folders.Folders {
		if folder.Status == QUEUED || folder.Status == EXTRACTING {
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
