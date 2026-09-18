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
	// SpeedBps is the current archive rate over the last sample interval.
	SpeedBps uint64
	// AvgSpeedBps is (completed archive bytes + current have) / extract duration.
	AvgSpeedBps uint64
	// ETA is when the current archive should finish, based on AvgSpeedBps.
	ETA         time.Time
	startedAt   time.Time
	sampleAt    time.Time
	sampleBytes uint64
	doneBytes   uint64
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

func bytesPerSec(delta uint64, elapsed time.Duration) uint64 {
	if elapsed <= 0 {
		return 0
	}

	return uint64(float64(delta) / elapsed.Seconds())
}

// ResetSpeed clears speed samples for a new extract.
func (p *Progress) ResetSpeed() {
	if p == nil {
		return
	}

	p.SpeedBps = 0
	p.AvgSpeedBps = 0
	p.ETA = time.Time{}
	p.startedAt = time.Time{}
	p.sampleAt = time.Time{}
	p.sampleBytes = 0
	p.doneBytes = 0
}

// NoteArchiveDone folds the finished archive into the extract total and
// clears the current-speed sample so the next archive starts a new interval.
func (p *Progress) NoteArchiveDone() {
	if p == nil {
		return
	}

	have, _ := progressBytes(p.Progress)
	p.doneBytes += have
	p.SpeedBps = 0
	p.sampleAt = time.Time{}
	p.sampleBytes = 0
}

// NoteSpeed updates current/average speeds and ETA from byte counts.
func (p *Progress) NoteSpeed(now time.Time) {
	if p == nil || p.Progress == nil {
		return
	}

	have, want := progressBytes(p.Progress)
	p.updateSpeeds(now, have)
	p.updateETA(now, have, want)
}

func (p *Progress) updateSpeeds(now time.Time, have uint64) {
	if have < p.sampleBytes {
		p.SpeedBps = 0
		p.sampleAt = now
		p.sampleBytes = have
	}

	if p.startedAt.IsZero() {
		p.startedAt = now
	}

	if p.sampleAt.IsZero() {
		p.sampleAt = now
		p.sampleBytes = have
	}

	if elapsed := now.Sub(p.startedAt); elapsed >= speedSampleInterval {
		p.AvgSpeedBps = bytesPerSec(p.doneBytes+have, elapsed)
	}

	if dt := now.Sub(p.sampleAt); dt >= speedSampleInterval {
		if have > p.sampleBytes {
			p.SpeedBps = bytesPerSec(have-p.sampleBytes, dt)
		} else {
			p.SpeedBps = 0
		}

		p.sampleAt = now
		p.sampleBytes = have
	}
}

func (p *Progress) updateETA(now time.Time, have, want uint64) {
	if p.AvgSpeedBps == 0 || want == 0 || have >= want {
		p.ETA = time.Time{}
		return
	}

	left := time.Duration(float64(want-have) / float64(p.AvgSpeedBps) * float64(time.Second))
	if left > maxETA {
		p.ETA = time.Time{}
		return
	}

	p.ETA = now.Add(left.Round(time.Second))
}
