package unpackerr

import (
	"errors"
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

func (u *Unpackerr) validateRadarr() error {
	tmp := u.Radarr[:0]

	for idx := range u.Radarr {
		if err := u.validateApp(&u.Radarr[idx].StarrConfig, starr.Radarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		u.Radarr[idx].Radarr = radarr.New(&u.Radarr[idx].Config)
		tmp = append(tmp, u.Radarr[idx])
	}

	u.Radarr = tmp

	return nil
}

func (u *Unpackerr) logRadarr() {
	if count := len(u.Radarr); count == 1 {
		u.Printf(" => Radarr Config: 1 server: "+starrLogLine,
			u.Radarr[0].URL, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout.String(),
			u.Radarr[0].ValidSSL, u.Radarr[0].Protocols, u.Radarr[0].Syncthing,
			u.Radarr[0].DeleteOrig, u.Radarr[0].DeleteDelay.String(), u.Radarr[0].Paths)
	} else {
		u.Printf(" => Radarr Config: %d servers", count)

		for _, f := range u.Radarr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout.String(), f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.String(), f.Paths)
		}
	}
}

// getRadarrQueue saves the Radarr Queue(s).
func (u *Unpackerr) getRadarrQueue(server *RadarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Radarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Radarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Radarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Radarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkRadarrQueue saves completed Radarr-queued downloads to u.Map.
func (u *Unpackerr) checkRadarrQueue(now time.Time) {
	for _, server := range u.Radarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Radarr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Map[record.Title] = &Extract{ // Save the download to our map.
					App:         starr.Radarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
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

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveRadarrQitem(name string) bool {
	for _, server := range u.Radarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			if record.Title == name {
				return true
			}
		}
	}

	return false
}
