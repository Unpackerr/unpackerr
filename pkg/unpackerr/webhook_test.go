package unpackerr

import (
	"errors"
	"testing"
	"time"

	"golift.io/starr"
	"golift.io/xtractr"
)

var errHookExtract = errors.New("boom")

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

func TestHookPayloadKeepsDataForImportedAndDeleted(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	resp := &xtractr.Response{
		NewFiles: []string{"/out/a.mkv"},
		Output:   "/tmp/out",
		Size:     42,
		Queued:   3,
		Started:  started,
		Elapsed:  time.Second,
		Archives: xtractr.ArchiveList{"/dl": {"/dl/a.rar"}},
		Error:    errHookExtract,
	}

	for _, status := range []ExtractStatus{IMPORTED, DELETED} {
		payload := hookPayload(&Extract{
			App:    starr.Sonarr,
			Path:   "/dl",
			Status: status,
			Resp:   resp,
		})
		if payload.Data == nil {
			t.Fatalf("%s: data is nil", status)
		}

		if payload.Data.Bytes != 42 || payload.Data.Output != "/tmp/out" || payload.Data.Queue != 3 {
			t.Fatalf("%s: %+v", status, payload.Data)
		}

		if payload.Data.Error != errHookExtract.Error() {
			t.Fatalf("%s: error %q", status, payload.Data.Error)
		}

		if len(payload.Data.File) != 1 || payload.Data.File[0] != "/out/a.mkv" {
			t.Fatalf("%s: files %+v", status, payload.Data.File)
		}

		if len(payload.Data.Archives) != 1 || payload.Data.Archives[0] != "/dl/a.rar" {
			t.Fatalf("%s: archives %+v", status, payload.Data.Archives)
		}
	}
}

func TestHookPayloadOmitsDataWithoutResp(t *testing.T) {
	t.Parallel()

	payload := hookPayload(&Extract{App: starr.Sonarr, Path: "/dl", Status: IMPORTED})
	if payload.Data != nil {
		t.Fatalf("data = %+v", payload.Data)
	}
}
