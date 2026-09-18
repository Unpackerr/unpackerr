package unpackerr

import (
	"testing"
	"time"

	"golift.io/xtractr"
)

func TestResetExtractProgressClearsRetryTelemetry(t *testing.T) {
	t.Parallel()

	item := &Extract{Path: "/dl"}
	item.XProg = &ExtractProgress{Extract: item, Extracted: 3, Archives: 4}
	item.XProg.Progress = &xtractr.Progress{Wrote: 500, Total: 500}
	start := time.Unix(1_700_000_000, 0)
	item.XProg.NoteSpeed(start)
	item.XProg.NoteSpeed(start.Add(time.Second))
	item.XProg.NoteArchiveDone()

	resetExtractProgress(item, 2)

	if item.XProg.Extracted != 0 || item.XProg.Archives != 2 || item.XProg.Progress != nil {
		t.Fatalf("progress %+v extracted %d archives %d",
			item.XProg.Progress, item.XProg.Extracted, item.XProg.Archives)
	}

	item.XProg.Progress = &xtractr.Progress{Wrote: 100, Total: 1000}
	item.XProg.NoteSpeed(start)
	item.XProg.NoteSpeed(start.Add(time.Second))

	if item.XProg.AvgSpeedBps != 100 {
		t.Fatalf("avg after retry %d", item.XProg.AvgSpeedBps)
	}
}
