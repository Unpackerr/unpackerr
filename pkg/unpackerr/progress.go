package unpackerr

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/xtractr"
)

const (
	minimumProgressInterval = time.Second
	defaultProgressInterval = 15 * time.Second
)

// ExtractProgress holds the progress for an entire Extract.
// An Extract is "a new item in a watch folder" or "a download in a starr app".
// Either may produce multiple xtractr.XFile structs (extractable archives).
type ExtractProgress struct {
	*xtractr.Progress
	// Extract extract that exists in the map.
	*Extract
	// Number of archives in this Xtract.
	Archives int
	// Number of archives extracted from this Xtract.
	Extracted int
}

func (p *ExtractProgress) String() string {
	if p == nil || p.Progress == nil {
		return "no progress yet"
	}

	var wrote, total uint64

	if p.Total > 0 {
		wrote, total = p.Wrote, p.Total
	} else if p.Compressed > 0 {
		wrote, total = p.Read, p.Compressed
	}

	return fmt.Sprintf("on archive: %d/%d @ %sB/%sB (%.0f%%): %s",
		p.Extracted+1, p.Archives, bytefmt.ByteSize(wrote), bytefmt.ByteSize(total),
		p.Percent(), strings.TrimLeft(strings.TrimPrefix(p.XFile.FilePath, p.Path), string(filepath.Separator)))
}

func (u *Unpackerr) progressUpdateCallback(item *Extract) func(xtractr.Progress) {
	return func(prog xtractr.Progress) { // sends update to u.handleProgress() (below)
		u.progChan <- &ExtractProgress{Progress: &prog, Extract: item}
	}
}

// prog = what just came in, it's ephemeral.
// prog.XProg = what is saved in the map, update this one.
// prog.Progress = also what just came in, we just set it here.
func (u *Unpackerr) handleProgress(prog *ExtractProgress) {
	if prog.XProg.Progress != nil && prog.XProg.XFile != prog.XFile {
		prog.XProg.Extracted++
	}

	prog.XProg.Progress = prog.Progress
}

func (u *Unpackerr) printProgress(now time.Time) {
	for name, data := range u.Map {
		if data.Status != EXTRACTING {
			continue
		}

		if prog := data.XProg.String(); prog != "no progress yet" {
			u.Printf("[%s] Status: %s (%v, elapsed: %v) %s", data.App, name, data.Status.Desc(),
				now.Sub(data.Updated).Round(time.Second), prog)
		}
	}
}
