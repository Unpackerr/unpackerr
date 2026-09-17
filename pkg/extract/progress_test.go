package extract

import (
	"testing"
	"time"

	"golift.io/xtractr"
)

func TestNoteSpeedSetsETAFromAverage(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Total: 1000, Wrote: 100}}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.Wrote = 300
	prog.NoteSpeed(start.Add(time.Second))

	if prog.AvgSpeedBps != 300 {
		t.Fatalf("avg %d", prog.AvgSpeedBps)
	}

	if prog.SpeedBps != 200 {
		t.Fatalf("current %d", prog.SpeedBps)
	}

	// Remaining 700 B at 300 B/s ≈ 2s after the second sample.
	wantETA := start.Add(3 * time.Second)
	if prog.ETA.IsZero() || prog.ETA.Before(wantETA.Add(-time.Second)) || prog.ETA.After(wantETA.Add(time.Second)) {
		t.Fatalf("eta %v avg %d", prog.ETA, prog.AvgSpeedBps)
	}
}

func TestNoteSpeedCurrentIsLastInterval(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Total: 2000, Wrote: 0}}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.Wrote = 100
	prog.NoteSpeed(start.Add(time.Second))
	prog.Wrote = 300
	prog.NoteSpeed(start.Add(time.Second + 200*time.Millisecond))

	if prog.SpeedBps != 1000 {
		t.Fatalf("current %d", prog.SpeedBps)
	}

	if prog.AvgSpeedBps < 240 || prog.AvgSpeedBps > 260 {
		t.Fatalf("avg %d", prog.AvgSpeedBps)
	}
}

func TestNoteSpeedRewindClearsCurrent(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Compressed: 800, Read: 400}, SpeedBps: 50}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.Read = 10
	prog.NoteSpeed(start.Add(time.Second))

	if prog.SpeedBps != 0 {
		t.Fatalf("current %+v", prog)
	}

	if prog.AvgSpeedBps != 10 {
		t.Fatalf("avg %d", prog.AvgSpeedBps)
	}
}

func TestNoteArchiveDoneKeepsAverage(t *testing.T) {
	t.Parallel()

	prog := &Progress{Progress: &xtractr.Progress{Total: 500, Wrote: 500}}
	start := time.Unix(1_700_000_000, 0)
	prog.NoteSpeed(start)
	prog.NoteSpeed(start.Add(time.Second))
	prog.NoteArchiveDone()

	prog.Progress = &xtractr.Progress{Total: 1000, Wrote: 0}
	prog.NoteSpeed(start.Add(time.Second))
	prog.Wrote = 200
	prog.NoteSpeed(start.Add(2 * time.Second))

	if prog.doneBytes != 500 {
		t.Fatalf("done %d", prog.doneBytes)
	}

	if prog.AvgSpeedBps != 350 {
		t.Fatalf("avg %d", prog.AvgSpeedBps)
	}

	if prog.SpeedBps != 200 {
		t.Fatalf("current %d", prog.SpeedBps)
	}
}
