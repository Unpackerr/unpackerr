package unpackerr

import (
	"fmt"
	"time"

	"golift.io/starr"
	"golift.io/starr/radarr"
)

// RadarrConfig represents the input data for a Radarr server.
type RadarrConfig struct {
	StarrConfig
	Queue          *radarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*radarr.Radarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (r *RadarrConfig) pollQueue() (int, int, error) {
	queue, err := r.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		return 0, 0, fmt.Errorf("getting queue: %w", err)
	}

	r.Queue = queue

	return queue.TotalRecords, len(queue.Records), nil
}

func (r *RadarrConfig) queueViews() []queueView {
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
				"downloadId": rec.DownloadID,
				"title":      rec.Title,
				"movieId":    rec.MovieID,
				"reason":     buildStatusReason(rec.Status, rec.StatusMessages),
			},
		})
	}

	return out
}

func (r *RadarrConfig) hasQueueTitle(name string) bool {
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

// checkRadarrQueue saves completed Radarr-queued downloads to u.Map.
func (u *Unpackerr) checkRadarrQueue(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for _, server := range u.Radarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Radarr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols) && !u.isForgotten(record.Title):
				u.Map[record.Title] = &Extract{ // Save the download to our map.
					App:         starr.Radarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					MaxBytes:    server.maxBytes,
					Path:        u.getDownloadPath(record.OutputPath, starr.Radarr, record.Title, server.Paths),
					IDs: map[string]any{
						"downloadId": record.DownloadID,
						"title":      record.Title,
						"movieId":    record.MovieID,
						"reason":     buildStatusReason(record.Status, record.StatusMessages),
					},
				}
				u.Map[record.Title].XProg = &ExtractProgress{Extract: u.Map[record.Title]}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Radarr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title)
			}
		}
	}
}
