package unpackerr

import (
	"time"

	"golift.io/xtractr"
)

const (
	minimumProgressInterval = time.Second
	defaultProgressInterval = 15 * time.Second
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
	if exp == nil || exp.XProg == nil {
		return
	}

	u.lockHistory()
	defer u.unlockHistory()

	if exp.XProg.Progress != nil && exp.XProg.XFile != exp.XFile {
		exp.XProg.Extracted++
		exp.XProg.NoteArchiveDone()
	}

	exp.XProg.Progress = exp.Progress
	exp.XProg.NoteSpeed(time.Now())

	if u.hub != nil && exp.Extract != nil {
		itemID := exp.Path
		for name, item := range u.Map {
			if item == exp.Extract {
				itemID = name
				break
			}
		}

		u.hub.notifyProgress(u.queueFromExtract(itemID, exp.Extract))
	}
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
