package unpackerr

import (
	"fmt"
	"time"

	"golift.io/starr"
	"golift.io/starr/sonarr"
)

// SonarrConfig represents the input data for a Sonarr server.
type SonarrConfig struct {
	StarrConfig
	Queue          *sonarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*sonarr.Sonarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (s *SonarrConfig) pollQueue() (int, int, error) {
	queue, err := s.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		return 0, 0, fmt.Errorf("getting queue: %w", err)
	}

	s.Queue = queue

	return queue.TotalRecords, len(queue.Records), nil
}

func (s *SonarrConfig) queueViews() []queueView {
	if s.Queue == nil {
		return nil
	}

	out := make([]queueView, 0, len(s.Queue.Records))

	for _, rec := range s.Queue.Records {
		out = append(out, queueView{
			Title:      rec.Title,
			Status:     rec.Status,
			Protocol:   rec.Protocol,
			OutputPath: rec.OutputPath,
			Size:       rec.Size,
			Sizeleft:   rec.Sizeleft,
			DebugExtra: fmt.Sprintf(" (Ep: %v)", rec.EpisodeID),
			IDs: map[string]any{
				"title":      rec.Title,
				"downloadId": rec.DownloadID,
				"seriesId":   rec.SeriesID,
				"episodeId":  rec.EpisodeID,
				"reason":     buildStatusReason(rec.Status, rec.StatusMessages),
			},
		})
	}

	return out
}

// checkSonarrQueue saves completed Sonarr-queued downloads to u.Map.
func (u *Unpackerr) checkSonarrQueue(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for _, server := range u.Sonarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import: %v", starr.Sonarr, server.URL, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols) && !u.isForgotten(record.Title):
				u.Map[record.Title] = &Extract{
					App:         starr.Sonarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					MaxBytes:    server.maxBytes,
					Path:        u.getDownloadPath(record.OutputPath, starr.Sonarr, record.Title, server.Paths),
					IDs: map[string]any{
						"title":      record.Title,
						"downloadId": record.DownloadID,
						"seriesId":   record.SeriesID,
						"episodeId":  record.EpisodeID,
						"reason":     buildStatusReason(record.Status, record.StatusMessages),
					},
				}
				u.Map[record.Title].XProg = &ExtractProgress{Extract: u.Map[record.Title]}

				fallthrough
			default:
				u.Debugf("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
					starr.Sonarr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title, record.EpisodeID)
			}
		}
	}
}
