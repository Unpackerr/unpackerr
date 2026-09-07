package unpackerr

import "testing"

func TestIdleBlocksOnInFlightWork(t *testing.T) {
	t.Parallel()

	unpack := New()
	if !unpack.idle() {
		t.Fatal("empty queue must be idle")
	}

	unpack.Map["/dl/waiting"] = &Extract{Path: "/dl/waiting", Status: WAITING}
	unpack.Map["/dl/failed"] = &Extract{Path: "/dl/failed", Status: EXTRACTFAILED}

	if !unpack.idle() {
		t.Fatal("waiting and failed items are rediscovered after a restart; they must not block")
	}

	for _, status := range []ExtractStatus{QUEUED, EXTRACTING, EXTRACTED, IMPORTED, DELETING} {
		unpack.Map["/dl/busy"] = &Extract{Path: "/dl/busy", Status: status}

		if unpack.idle() {
			t.Fatalf("%s must block a restart", status)
		}
	}

	delete(unpack.Map, "/dl/busy")
	unpack.folders.Folders["/watch/x"] = &Folder{status: EXTRACTING}

	if unpack.idle() {
		t.Fatal("an extracting folder must block a restart")
	}

	unpack.folders.Folders["/watch/x"].status = WAITING
	unpack.queueDelete(&fileDeleteReq{})

	if unpack.idle() {
		t.Fatal("a pending delete must block a restart")
	}
}

func TestMaybeRestartWaitsForIdle(t *testing.T) {
	t.Parallel()

	unpack := New()
	unpack.pendingRestart = true
	unpack.Map["/dl/busy"] = &Extract{Path: "/dl/busy", Status: EXTRACTING}

	unpack.maybeRestart()

	if !unpack.pendingRestart {
		t.Fatal("restart must stay pending while an extraction runs")
	}
}
