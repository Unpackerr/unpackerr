package unpackerr

import (
	"fmt"

	"golift.io/starr/sonarr"
)

// SonarrConfig represents the input data for a Sonarr server.
type SonarrConfig struct {
	StarrConfig
	Queue          *sonarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*sonarr.Sonarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (s *SonarrConfig) pollQueue() (int, int, error) {
	if s.Sonarr == nil {
		s.connect()
	}

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

func (s *SonarrConfig) hasQueueTitle(name string) bool {
	if s.Queue == nil {
		return false
	}

	for _, rec := range s.Queue.Records {
		if rec.Title == name {
			return true
		}
	}

	return false
}

func (*SonarrConfig) tweakExtract(_ *Extract, _ queueView) {}

func (*SonarrConfig) logExtra() string { return "" }
