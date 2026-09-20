package hooks

import (
	"fmt"
	"runtime"
	"time"

	"github.com/Unpackerr/unpackerr/pkg/extract"
	"golift.io/cnfg"
	"golift.io/version"
	"golift.io/xtractr"
)

const sampleRetries = 2

// SamplePayload is a fake payload used by `unpackerr -w`.
func SamplePayload() *Payload {
	return &Payload{
		App:  "Starr",
		Path: "/this/is/a/path",
		IDs: map[string]any{
			"title":      "Some Cool Title Name Here",
			"downloadId": fmt.Sprintf("some-id-goes-here-%d", time.Now().Unix()),
			"otherId":    "another-id-here-like-imdb",
		},
		CustomIDs: map[string]string{
			"url": "https://unpackerr.example",
		},
		Time:       time.Now(),
		Retries:    sampleRetries,
		EventTitle: extract.EXTRACTING.Desc(),
		Go:         runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Version:    version.Version,
		Revision:   version.Revision,
		Branch:     version.Branch,
		Started:    version.Started,
		Data: &XtractPayload{
			Start:    version.Started,
			Elapsed:  cnfg.Duration{Duration: time.Since(version.Started)},
			Archives: []string{"/this/is/the/extraction/path/archive.rar", "/this/is/the/extraction/path/archive.sub.rar"},
			Error:    "This is where an error goes.",
			Output:   "/this/is/the/extraction/path_unpackerred",
			Bytes:    0,
			Files:    []string{"/this/is/the/extraction/path/file.mkv", "/this/is/the/extraction/path/file.sub"},
		},
	}
}

// PrepareSample mutates a sample payload for the requested event.
func PrepareSample(payload *Payload, event extract.Status) error {
	payload.Event = event
	payload.EventTitle = event.Desc()

	if payload.Retries == 0 {
		payload.Retries = sampleRetries
	}

	switch event {
	default:
		return fmt.Errorf("%w: %s", ErrUnknownEvent, event)
	case extract.WAITING:
		payload.Data = nil
	case extract.QUEUED:
		payload.App = "Folder"
		payload.Data = nil
	case extract.EXTRACTING:
		payload.Data.Bytes = 0
		payload.Data.Files = nil
		payload.Data.Elapsed.Duration = 0
	case extract.EXTRACTED:
		payload.Data.Bytes = 1234567009
	case extract.EXTRACTFAILED:
		payload.Data.Files = nil
		payload.Data.Bytes = 0
		payload.Data.Error = xtractr.ErrInvalidHead.Error()
	case extract.IMPORTED:
		payload.Data.Bytes = 0
		payload.Data.Files = nil
	case extract.DELETING:
		payload.Data.Elapsed.Duration = 0
	case extract.DELETED:
		payload.Data.Elapsed.Duration = 0
	case extract.DELETEFAILED:
		payload.Data.Elapsed.Duration = 0
		payload.Data.Error = "unable to delete files"
	case extract.EXTRACTEDNOTHING:
		payload.Data.Files = nil
		payload.Data.Archives = nil
		payload.Data.Bytes = 0
		payload.Data.Error = ""
	}

	return nil
}
