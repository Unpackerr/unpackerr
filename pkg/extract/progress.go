package extract

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/xtractr"
)

const (
	speedSampleInterval = 200 * time.Millisecond
	maxETA              = 24 * time.Hour
	speedSmoothDiv      = 4
)

// Progress holds the progress for an entire Extract.
// An Extract is "a new item in a watch folder" or "a download in a starr app".
// Either may produce multiple xtractr.XFile structs (extractable archives).
type Progress struct {
	*xtractr.Progress
	// Extract that exists in the map.
	*Extract
	// Number of archives in this Extract.
	Archives int
	// Number of archives extracted from this Extract.
	Extracted int
	// SpeedBps is a smoothed bytes/sec for the current archive.
	SpeedBps uint64
	// ETA is when the current archive should finish, based on SpeedBps.
	ETA         time.Time
	sampleAt    time.Time
	sampleBytes uint64
}

func (p *Progress) String() string {
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

func progressBytes(prog *xtractr.Progress) (uint64, uint64) {
	if prog == nil {
		return 0, 0
	}

	if prog.Total > 0 {
		return prog.Wrote, prog.Total
	}

	if prog.Compressed > 0 {
		return prog.Read, prog.Compressed
	}

	return 0, 0
}

// ResetSpeed clears the current-archive speed sample (new archive or rewind).
func (p *Progress) ResetSpeed() {
	if p == nil {
		return
	}

	p.SpeedBps = 0
	p.ETA = time.Time{}
	p.sampleAt = time.Time{}
	p.sampleBytes = 0
}

// NoteSpeed updates SpeedBps/ETA from the current archive byte counts.
func (p *Progress) NoteSpeed(now time.Time) {
	if p == nil || p.Progress == nil {
		return
	}

	have, want := progressBytes(p.Progress)
	if want == 0 {
		p.ETA = time.Time{}
		return
	}

	if have < p.sampleBytes {
		p.ResetSpeed()
	}

	if p.sampleAt.IsZero() {
		p.sampleAt = now
		p.sampleBytes = have

		return
	}

	if dt := now.Sub(p.sampleAt); dt >= speedSampleInterval && have > p.sampleBytes {
		inst := uint64(float64(have-p.sampleBytes) / dt.Seconds())
		if p.SpeedBps == 0 {
			p.SpeedBps = inst
		} else {
			p.SpeedBps = (p.SpeedBps*(speedSmoothDiv-1) + inst) / speedSmoothDiv
		}

		p.sampleAt = now
		p.sampleBytes = have
	}

	if p.SpeedBps == 0 || have >= want {
		p.ETA = time.Time{}
		return
	}

	left := time.Duration(float64(want-have) / float64(p.SpeedBps) * float64(time.Second))
	if left > maxETA {
		p.ETA = time.Time{}
		return
	}

	p.ETA = now.Add(left.Round(time.Second))
}
