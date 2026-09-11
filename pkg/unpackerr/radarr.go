package unpackerr

import (
	"fmt"

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

func (*RadarrConfig) tweakExtract(_ *Extract, _ queueView) {}
