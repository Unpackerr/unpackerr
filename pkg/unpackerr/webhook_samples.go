package unpackerr

import (
	"time"

	"golift.io/version"
	"golift.io/xtractr"
)

func (u *Unpackerr) sampleWebhook(e ExtractStatus) error {
	u.Logf("Sending webhooks.")

	if e == WAITING || e > DELETED {
		return ErrInvalidStatus
	}

	payload := &Extract{
		App:  Sonarr,
		Path: "/this/is/the/extraction/path",
		IDs: map[string]interface{}{
			"downloadId": "some-id-goes-here",
			"otherId":    "another-id-here-like-imdb",
		},
		Status:  e,
		Updated: time.Now(),
		Resp: &xtractr.Response{
			Elapsed:  time.Since(version.Started),
			Extras:   nil,
			Archives: []string{"/this/is/the/extraction/path/archive.rar"},
			AllFiles: nil,
			Error:    nil,
			X: &xtractr.Xtract{
				Name:       "path",
				SearchPath: "/this/is/the/extraction/path",
				TempFolder: true,
				DeleteOrig: false,
				CBFunction: nil,
				CBChannel:  nil,
			},
			Started:  version.Started,
			Done:     e >= EXTRACTED,
			Output:   "/this/is/the/extraction/path_unpackerred",
			Queued:   0,
			NewFiles: nil,
			Size:     0,
		},
	}

	if e != EXTRACTING && e != EXTRACTED && e != EXTRACTFAILED {
		payload.Resp = nil
	}

	if e == QUEUED {
		payload.App = "Folder"
	}

	if e == EXTRACTFAILED {
		payload.Resp.Error = xtractr.ErrInvalidHead
	}

	for _, hook := range u.Webhook {
		u.sendWebhookWithLog(hook, payload)
	}

	return nil
}
