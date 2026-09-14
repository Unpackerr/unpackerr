package extract

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"golift.io/xtractr"
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
	// StartedAt is when the current archive began extracting.
	StartedAt time.Time
	// UpdatedAt is when the current progress counters were last refreshed.
	UpdatedAt time.Time
}

func (p *Progress) String() string {
	if p == nil || p.Progress == nil {
		return "no progress yet"
	}

	wrote, total := p.Bytes()

	return fmt.Sprintf("on archive: %d/%d @ %sB/%sB (%.0f%%): %s",
		p.Extracted+1, p.Archives, bytefmt.ByteSize(wrote), bytefmt.ByteSize(total),
		p.Percent(), strings.TrimLeft(strings.TrimPrefix(p.XFile.FilePath, p.Path), string(filepath.Separator)))
}

func (p *Progress) Bytes() (uint64, uint64) {
	if p == nil || p.Progress == nil {
		return 0, 0
	}

	if p.Total > 0 {
		return p.Wrote, p.Total
	}

	if p.Compressed > 0 {
		return p.Read, p.Compressed
	}

	return 0, 0
}

func (p *Progress) Speed(now time.Time) (uint64, bool) {
	if p == nil || p.Progress == nil || p.StartedAt.IsZero() {
		return 0, false
	}

	elapsed := p.UpdatedAt.Sub(p.StartedAt)
	if elapsed <= 0 && now.After(p.StartedAt) {
		elapsed = now.Sub(p.StartedAt)
	}

	if elapsed < time.Second {
		return 0, false
	}

	wrote, _ := p.Bytes()
	if wrote == 0 {
		return 0, false
	}

	speed := uint64(float64(wrote) / elapsed.Seconds())

	return speed, speed > 0
}

func (p *Progress) ETA(now time.Time) (time.Duration, bool) {
	speed, ok := p.Speed(now)
	if !ok {
		return 0, false
	}

	wrote, total := p.Bytes()
	if total == 0 || wrote >= total {
		return 0, false
	}

	remaining := total - wrote

	eta := time.Duration(float64(remaining) / float64(speed) * float64(time.Second)).Round(time.Second)
	if eta > 0 && eta < time.Second {
		eta = time.Second
	}

	return eta, eta > 0
}

func (p *Progress) ProgressStartedAt(now time.Time) time.Time {
	if p != nil && p.Extract != nil && p.Resp != nil && !p.Resp.Started.IsZero() &&
		!p.Resp.Started.After(now) {
		return p.Resp.Started
	}

	if p != nil && p.Extract != nil && p.Status == EXTRACTING && !p.Updated.IsZero() &&
		!p.Updated.After(now) {
		return p.Updated
	}

	return now
}
