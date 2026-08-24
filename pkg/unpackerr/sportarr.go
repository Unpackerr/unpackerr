package unpackerr

import (
	"errors"
	"time"

	"golift.io/starr"
	"golift.io/starr/sonarr"
)

// Sportarr is not part of the starr library. It exposes a Sonarr-v3-compatible
// API, so the starr Sonarr client drives it; this constant names it in logs,
// metrics and webhooks.
const Sportarr starr.App = "Sportarr"

// SportarrConfig just uses sonarr.

func (u *Unpackerr) validateSportarr() error {
	tmp := u.Sportarr[:0]

	for idx := range u.Sportarr {
		if err := u.validateApp(&u.Sportarr[idx].StarrConfig, Sportarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		// shoehorned into Sonarr!
		u.Sportarr[idx].Sonarr = sonarr.New(&u.Sportarr[idx].Config)
		tmp = append(tmp, u.Sportarr[idx])
	}

	u.Sportarr = tmp

	return nil
}

func (u *Unpackerr) logSportarr() {
	if count := len(u.Sportarr); count == 1 {
		u.Printf(" => Sportarr Config: 1 server: "+starrLogLine,
			u.Sportarr[0].URL, u.Sportarr[0].APIKey != "", u.Sportarr[0].Timeout,
			u.Sportarr[0].ValidSSL, u.Sportarr[0].Protocols, u.Sportarr[0].Syncthing,
			u.Sportarr[0].DeleteOrig, u.Sportarr[0].DeleteDelay.Duration, u.Sportarr[0].Paths)
	} else if count != 0 {
		u.Printf(" => Sportarr Config: %d servers", count)

		for _, f := range u.Sportarr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getSportarrQueue saves the Sportarr Queue(s).
func (u *Unpackerr) getSportarrQueue(server *SonarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Sportarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		u.saveQueueMetrics(0, start, Sportarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, Sportarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Sportarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkSportarrQueue saves completed Sportarr-queued downloads to u.Map.
func (u *Unpackerr) checkSportarrQueue(now time.Time) {
	for _, server := range u.Sportarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import: %v", Sportarr, server.URL, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Map[record.Title] = &Extract{
					App:         Sportarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(record.OutputPath, Sportarr, record.Title, server.Paths),
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
					Sportarr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title, record.EpisodeID)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveSportarrQitem(name string) bool {
	for _, server := range u.Sportarr {
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
