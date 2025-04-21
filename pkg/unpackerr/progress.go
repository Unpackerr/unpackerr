package unpackerr

import (
	"fmt"
	"path/filepath"
	"time"

	"golift.io/xtractr"
)

const (
	minimumProgressInterval = 2 * time.Second
	defaultProgressInterval = 15 * time.Second
)

type Progress struct {
	*xtractr.Progress
	// Name of this item in the Map.
	Name string
	// Number of archives in this Xtract.
	Archives int
	// Number of archives extracted from this Xtract.
	Extracted int
}

func (p *Progress) String() string {
	if p == nil {
		return "no progress yet"
	}

	var wrote, total uint64

	if p.Total > 0 {
		wrote, total = p.Wrote, p.Total
	} else if p.Compressed > 0 {
		wrote, total = p.Read, p.Compressed
	}

	return fmt.Sprintf("extracted %d/%d archives, current: %d/%d bytes (%.0f%%): %s",
		p.Extracted, p.Archives, wrote, total, p.Percent(), filepath.Base(p.XFile.FilePath))
}

func (u *Unpackerr) progressUpdateCallback(name string) func(xtractr.Progress) {
	return func(prog xtractr.Progress) {
		u.progress <- &Progress{Progress: &prog, Name: name} // ends up in u.handleProgress() (below)
	}
}

func (u *Unpackerr) handleProgress(prog *Progress) {
	if item := u.Map[prog.Name]; item != nil {
		if item.Progress == nil {
			item.Progress = prog
		} else {
			item.Progress.Progress = prog.Progress
		}

		if prog.Done {
			item.Progress.Extracted++
		}

		if item.Resp != nil {
			prog.Archives = item.Resp.Archives.Count()
		}
	}
}

func (u *Unpackerr) printProgress(now time.Time) {
	for name, data := range u.Map {
		if data.Status != EXTRACTING {
			continue
		}

		u.Printf("[%s] Status: %s (%v, elapsed: %v) %s", data.App, name, data.Status.Desc(),
			now.Sub(data.Updated).Round(time.Second), data.Progress)
	}
}
