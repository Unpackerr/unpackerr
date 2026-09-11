package unpackerr

import (
	"fmt"

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

func (*ReadarrConfig) tweakExtract(_ *Extract, _ queueView) {}
