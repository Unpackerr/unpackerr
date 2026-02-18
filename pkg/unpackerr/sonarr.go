package unpackerr

import (
	"errors"
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

func (u *Unpackerr) validateSonarr() error {
	tmp := u.Sonarr[:0]

	for idx := range u.Sonarr {
		if err := u.validateApp(&u.Sonarr[idx].StarrConfig, starr.Sonarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		u.Sonarr[idx].Sonarr = sonarr.New(&u.Sonarr[idx].Config)
		tmp = append(tmp, u.Sonarr[idx])
	}

	u.Sonarr = tmp

	return nil
}

func (u *Unpackerr) logSonarr() {
	if count := len(u.Sonarr); count == 1 {
		u.Printf(" => Sonarr Config: 1 server: "+starrLogLine,
			u.Sonarr[0].URL, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout.String(),
			u.Sonarr[0].ValidSSL, u.Sonarr[0].Protocols, u.Sonarr[0].Syncthing,
			u.Sonarr[0].DeleteOrig, u.Sonarr[0].DeleteDelay.String(), u.Sonarr[0].Paths)
	} else {
		u.Printf(" => Sonarr Config: %d servers", count)

		for _, f := range u.Sonarr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout.String(), f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.String(), f.Paths)
		}
	}
}

// getSonarrQueue saves the Sonarr Queue(s).
func (u *Unpackerr) getSonarrQueue(server *SonarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Sonarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Sonarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Sonarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Sonarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkSonarrQueue saves completed Sonarr-queued downloads to u.Map.
func (u *Unpackerr) checkSonarrQueue(now time.Time) {
	for _, server := range u.Sonarr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import: %v", starr.Sonarr, server.URL, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Map[record.Title] = &Extract{
					App:         starr.Sonarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
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

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveSonarrQitem(name string) bool {
	for _, server := range u.Sonarr {
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
