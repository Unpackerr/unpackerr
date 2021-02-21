package unpackerr

import (
	"runtime"
	"time"

	"golift.io/version"
	"golift.io/xtractr"
)

func (u *Unpackerr) sampleWebhook(e ExtractStatus) error {
	u.Printf("Sending webhooks.")

	if e == WAITING || e > DELETED {
		return ErrInvalidStatus
	}

	payload := &WebhookPayload{
		App:  Sonarr,
		Path: "/this/is/the/extraction/path",
		IDs: map[string]interface{}{
			"title":      "Some Cool Movie Name Here",
			"downloadId": "some-id-goes-here",
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
		Event:    e,
		Data: &XtractPayload{
			Start:    version.Started,
			Elapsed:  time.Since(version.Started).Seconds(),
			Archives: []string{"/this/is/the/extraction/path/archive.rar"},
			Error:    "",
			Output:   "/this/is/the/extraction/path_unpackerred",
			Bytes:    0,
			Files:    []string{"/this/is/the/extraction/path/file.mkv"},
		},
	}

	if e != EXTRACTING && e != EXTRACTED && e != EXTRACTFAILED {
		payload.Data = nil
	} else {
		payload.Data.Bytes = 1234567009
	}

	if e == QUEUED {
		payload.App = FolderString
	}

	if e == EXTRACTFAILED {
		payload.Data.Error = xtractr.ErrInvalidHead.Error()
	}

	for _, hook := range u.Webhook {
		u.sendWebhookWithLog(hook, payload)
	}

	return nil
}
