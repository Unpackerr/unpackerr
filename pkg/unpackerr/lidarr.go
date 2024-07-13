package unpackerr

import (
	"errors"
	"sync"
	"time"

	"golift.io/starr"
	"golift.io/starr/lidarr"
)

// LidarrConfig represents the input data for a Lidarr server.
type LidarrConfig struct {
	StarrConfig
	Queue          *lidarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*lidarr.Lidarr `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateLidarr() error {
	tmp := u.Lidarr[:0]

	for idx := range u.Lidarr {
		if err := u.validateApp(&u.Lidarr[idx].StarrConfig, starr.Lidarr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		u.Lidarr[idx].Lidarr = lidarr.New(&u.Lidarr[idx].Config)
		tmp = append(tmp, u.Lidarr[idx])
	}

	u.Lidarr = tmp

	return nil
}

func (u *Unpackerr) logLidarr() {
	if c := len(u.Lidarr); c == 1 {
		u.Printf(" => Lidarr Config: 1 server: "+starrLogLine,
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout,
			u.Lidarr[0].ValidSSL, u.Lidarr[0].Protocols, u.Lidarr[0].Syncthing,
			u.Lidarr[0].DeleteOrig, u.Lidarr[0].DeleteDelay.Duration, u.Lidarr[0].Paths)
	} else {
		u.Printf(" => Lidarr Config: %d servers", c)

		for _, f := range u.Lidarr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getLidarrQueue saves the Lidarr Queue(s).
func (u *Unpackerr) getLidarrQueue(server *LidarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Lidarr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, DefaultQueuePageSize)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Lidarr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Lidarr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Lidarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkLidarrQueue saves completed Lidarr-queued downloads to u.Map.
func (u *Unpackerr) checkLidarrQueue(now time.Time) {
	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Lidarr, server.URL, q.Protocol, q.Title)
			case !ok && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Map[q.Title] = &Extract{
					App:         starr.Lidarr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(q.OutputPath, starr.Lidarr, q.Title, server.Paths),
					IDs: map[string]any{
						"title":      q.Title,
						"artistId":   q.ArtistID,
						"albumId":    q.AlbumID,
						"downloadId": q.DownloadID,
						"reason":     buildStatusReason(q.Status, q.StatusMessages),
					},
				}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Lidarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveLidarrQitem(name string) bool {
	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
