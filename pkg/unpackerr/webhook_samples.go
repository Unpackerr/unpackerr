package unpackerr

import (
	"fmt"
	"runtime"
	"time"

	"golift.io/cnfg"
	"golift.io/version"
	"golift.io/xtractr"
)

func (u *Unpackerr) sampleWebhook(e ExtractStatus) error {
	u.Printf("Sending sample webhooks and exiting! (-w %d passed)", e)

	payload := samplePayload()
	switch payload.Event = e; payload.Event {
	default:
		fallthrough
	case WAITING:
		return ErrInvalidStatus
	case QUEUED:
		payload.App = FolderString
		payload.Data = nil
	case EXTRACTING:
		payload.Data.Bytes = 0
		payload.Data.Files = nil
		payload.Data.Elapsed.Duration = 0
	case EXTRACTED:
		payload.Data.Bytes = 1234567009
	case EXTRACTFAILED:
		payload.Data.Files = nil
		payload.Data.Bytes = 0
		payload.Data.Error = xtractr.ErrInvalidHead.Error()
	case IMPORTED:
		payload.Data.Bytes = 0
		payload.Data.Files = nil
	case DELETING:
		payload.Data.Elapsed.Duration = 0
	case DELETED:
		payload.Data.Elapsed.Duration = 0
	case DELETEFAILED:
		payload.Data.Elapsed.Duration = 0
		payload.Data.Error = "unable to delete files"
	}

	for _, hook := range u.Webhook {
		u.sendWebhookWithLog(hook, payload)
	}

	return nil
}

func samplePayload() *WebhookPayload {
	return &WebhookPayload{
		App:  "Starr",
		Path: "/this/is/a/path",
		IDs: map[string]any{
			"title":      "Some Cool Title Name Here",
			"downloadId": fmt.Sprintf("some-id-goes-here-%d", time.Now().Unix()),
			"otherId":    "another-id-here-like-imdb",
		},
		Time:     time.Now(),
		Go:       runtime.Version(),
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  version.Version,
		Revision: version.Revision,
		Branch:   version.Branch,
		Started:  version.Started,
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
