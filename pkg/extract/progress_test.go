package extract

import (
	"testing"
	"time"

	"golift.io/xtractr"
)

func TestNoteSpeedSetsETA(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Total: 1000, Wrote: 100}}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.Wrote = 300
	prog.NoteSpeed(start.Add(time.Second))

	if prog.SpeedBps < 150 || prog.SpeedBps > 250 {
		t.Fatalf("speed %d", prog.SpeedBps)
	}

	if prog.ETA.IsZero() || prog.ETA.Before(start.Add(3*time.Second)) || prog.ETA.After(start.Add(6*time.Second)) {
		t.Fatalf("eta %v speed %d", prog.ETA, prog.SpeedBps)
	}
}

func TestNoteSpeedResetsOnRewind(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Compressed: 800, Read: 400}, SpeedBps: 50}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.Read = 10
	prog.NoteSpeed(start.Add(time.Second))

	if prog.SpeedBps != 0 || !prog.ETA.IsZero() {
		t.Fatalf("rewind %+v", prog)
	}
}
