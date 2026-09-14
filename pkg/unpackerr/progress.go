package unpackerr

import (
	"time"

	"golift.io/xtractr"
)

const (
	minimumProgressInterval = time.Second
	defaultProgressInterval = 15 * time.Second
	noProgressText          = "no progress yet"
)

func (u *Unpackerr) progressUpdateCallback(item *Extract) func(xtractr.Progress) {
	return func(prog xtractr.Progress) { // sends update to u.handleProgress() (below)
		u.progChan <- &ExtractProgress{Progress: &prog, Extract: item}
	}
}

// exp = what just came in, it's ephemeral.
// exp.Progress = also what just came in, must set it here.
// exp.XProg = what is saved in the map, update this one.
func (u *Unpackerr) handleProgress(exp *ExtractProgress) {
	if exp == nil || exp.Extract == nil || exp.XProg == nil || exp.Progress == nil {
		return
	}

	u.lockHistory()
	defer u.unlockHistory()

	xprog := exp.XProg
	now := time.Now()

	if xprog.Progress != nil && xprog.XFile != exp.XFile {
		xprog.Extracted++
		xprog.StartedAt = now
	} else if xprog.StartedAt.IsZero() {
		xprog.StartedAt = exp.ProgressStartedAt(now)
	}

	xprog.Progress = exp.Progress
	xprog.UpdatedAt = now
}

func (u *Unpackerr) printProgress(now time.Time) {
	u.rLockHistory()
	defer u.rUnlockHistory()

	for name, data := range u.Map {
		if data.Status != EXTRACTING {
			continue
		}

		if prog := data.XProg.String(); prog != "no progress yet" {
			u.Printf("[%s] Status: %s (%v, elapsed: %v) %s", data.Label(), name, data.Status.Desc(),
				now.Sub(data.Updated).Round(time.Second), prog)
		}
	}
}
