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
	u.Printf("Sending webhooks.")

	if e == WAITING || e > DELETED {
		return ErrInvalidStatus
	}

	payload := &WebhookPayload{
		App:  Sonarr,
		Path: "/this/is/the/extraction-path/Some.Cool.Movie.Name.Here",
		IDs: map[string]interface{}{
			"title":      "Some Cool Movie Name Here",
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
		Event:    e,
		Data: &XtractPayload{
			Start:    version.Started,
			Elapsed:  cnfg.Duration{Duration: time.Since(version.Started)},
			Archives: []string{"/this/is/the/extraction/path/archive.rar", "/this/is/the/extraction/path/archive.sub.rar"},
			Error:    "",
			Output:   "/this/is/the/extraction/path_unpackerred",
			Bytes:    0,
			Files:    []string{"/this/is/the/extraction/path/file.mkv", "/this/is/the/extraction/path/file.sub"},
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
