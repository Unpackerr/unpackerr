package unpackerr

import (
	"fmt"
	"time"

	"golift.io/starr"
	"golift.io/starr/readarr"
)

// ReadarrConfig represents the input data for a Readarr server.
type ReadarrConfig struct {
	StarrConfig
	Queue            *readarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*readarr.Readarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (r *ReadarrConfig) pollQueue() (int, int, error) {
	queue, err := r.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		return 0, 0, fmt.Errorf("getting queue: %w", err)
	}

	r.Queue = queue

	return queue.TotalRecords, len(queue.Records), nil
}

func (r *ReadarrConfig) queueViews() []queueView {
	if r.Queue == nil {
		return nil
	}

	out := make([]queueView, 0, len(r.Queue.Records))

	for _, rec := range r.Queue.Records {
		out = append(out, queueView{
			Title:      rec.Title,
			Status:     rec.Status,
			Protocol:   rec.Protocol,
			OutputPath: rec.OutputPath,
			Size:       rec.Size,
			Sizeleft:   rec.Sizeleft,
			IDs: map[string]any{
				"title":      rec.Title,
				"authorId":   rec.AuthorID,
				"bookId":     rec.BookID,
				"downloadId": rec.DownloadID,
				"reason":     buildStatusReason(rec.Status, rec.StatusMessages),
			},
		})
	}

	return out
}

func (r *ReadarrConfig) hasQueueTitle(name string) bool {
	if r.Queue == nil {
		return false
	}

	for _, rec := range r.Queue.Records {
		if rec.Title == name {
			return true
		}
	}

	return false
}

// checkReadarQueue saves completed Readarr-queued downloads to u.Map.
func (u *Unpackerr) checkReadarrQueue(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for _, server := range u.Readarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Readarr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols) && !u.isForgotten(record.Title):
				u.Map[record.Title] = &Extract{
					App:         starr.Readarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					MaxBytes:    server.maxBytes,
					Path:        u.getDownloadPath(record.OutputPath, starr.Readarr, record.Title, server.Paths),
					IDs: map[string]any{
						"title":      record.Title,
						"authorId":   record.AuthorID,
						"bookId":     record.BookID,
						"downloadId": record.DownloadID,
						"reason":     buildStatusReason(record.Status, record.StatusMessages),
					},
				}
				u.Map[record.Title].XProg = &ExtractProgress{Extract: u.Map[record.Title]}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Readarr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title)
			}
		}
	}
}
