package unpackerr

import (
	"testing"

	"golift.io/starr"
)

func TestHookPayloadUsesLabel(t *testing.T) {
	t.Parallel()

	payload := hookPayload(&Extract{App: starr.Sonarr, Name: "Sportarr", Path: "/dl"})
	if payload.App != "Sportarr" {
		t.Fatalf("payload.App = %q, want Sportarr", payload.App)
	}

	plain := hookPayload(&Extract{App: starr.Sonarr, Path: "/dl"})
	if plain.App != starr.Sonarr {
		t.Fatalf("unnamed payload.App = %q, want %s", plain.App, starr.Sonarr)
	}
}
