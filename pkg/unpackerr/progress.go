package unpackerr

import (
	"maps"
	"time"

	"golift.io/xtractr"
)

const (
	minimumProgressInterval = time.Second
	defaultProgressInterval = 15 * time.Second
)

func resetExtractProgress(item *Extract, archives int) {
	if item == nil {
		return
	}

	if item.XProg == nil {
		item.XProg = &ExtractProgress{Extract: item}
	}

	item.XProg.ResetSpeed()
	item.XProg.Extracted = 0
	item.XProg.Progress = nil
	item.XProg.Archives = archives
	item.XProg.TempFiles = nil
}

// progressUpdateCallback runs in the extract's own goroutine. For a Syncthing
// item, the last update of each archive also lists the temp folder. The
// files are still there: they move into the download after the last archive.
// That list is how the files an extract wrote into a folder are told apart
// from a file Syncthing adds to that folder right after the move.
func (u *Unpackerr) progressUpdateCallback(item *Extract) func(xtractr.Progress) {
	syncthing := item.Syncthing

	return func(prog xtractr.Progress) { // sends update to u.handleProgress() (below)
		exp := &ExtractProgress{Progress: &prog, Extract: item}
		if syncthing && prog.Done && prog.XFile != nil {
			exp.TempFiles = tempFiles(prog.XFile.OutputDir)
		}

		u.progChan <- exp
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

	if exp.TempFiles != nil {
		// Each source folder is extracted and moved in turn, so the temp
		// folder starts over. Keep every list.
		if exp.XProg.TempFiles == nil {
			exp.XProg.TempFiles = make(map[string]struct{}, len(exp.TempFiles))
		}

		maps.Copy(exp.XProg.TempFiles, exp.TempFiles)
	}

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
