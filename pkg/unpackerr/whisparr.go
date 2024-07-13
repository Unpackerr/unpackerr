package unpackerr

import (
	"errors"
	"time"

	"golift.io/starr"
	"golift.io/starr/radarr"
)

// WhisparrConfig just uses radarr.
/*
type WhisparrConfig struct {
	starr.Config
	Path           string        `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths          []string      `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols      string        `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	DeleteOrig     bool          `json:"delete_orig" toml:"delete_orig" xml:"delete_orig" yaml:"delete_orig"`
	DeleteDelay    cnfg.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Queue          *whisparr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
	*whisparr.Whisparr `json:"-" toml:"-" xml:"-" yaml:"-"`
} */

func (u *Unpackerr) validateWhisparr() error {
	tmp := u.Whisparr[:0]

	for idx := range u.Whisparr {
		if err := u.validateApp(&u.Whisparr[idx].StarrConfig, starr.Whisparr); err != nil {
			if errors.Is(err, ErrInvalidURL) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		// shoehorned into Radarr!
		u.Whisparr[idx].Radarr = radarr.New(&u.Whisparr[idx].Config)
		tmp = append(tmp, u.Whisparr[idx])
	}

	u.Whisparr = tmp

	return nil
}

func (u *Unpackerr) logWhisparr() {
	if count := len(u.Whisparr); count == 1 {
		u.Printf(" => Whisparr Config: 1 server: "+starrLogLine,
			u.Whisparr[0].URL, u.Whisparr[0].APIKey != "", u.Whisparr[0].Timeout,
			u.Whisparr[0].ValidSSL, u.Whisparr[0].Protocols, u.Whisparr[0].Syncthing,
			u.Whisparr[0].DeleteOrig, u.Whisparr[0].DeleteDelay.Duration, u.Whisparr[0].Paths)
	} else if count != 0 {
		u.Printf(" => Whisparr Config: %d servers", count)

		for _, f := range u.Whisparr {
			u.Printf(starrLogPfx+starrLogLine,
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getWhisparrQueue saves the Whisparr Queue(s).
func (u *Unpackerr) getWhisparrQueue(server *RadarrConfig, start time.Time) {
	if server.APIKey == "" {
		u.Debugf("Whisparr (%s): skipped, no API key", server.URL)
		return
	}

	queue, err := server.GetQueue(DefaultQueuePageSize, 1)
	if err != nil {
		u.saveQueueMetrics(0, start, starr.Whisparr, server.URL, err)
		return
	}

	// Only update if there was not an error fetching.
	server.Queue = queue
	u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Whisparr, server.URL, nil)

	if !u.Activity || queue.TotalRecords > 0 {
		u.Printf("[Whisparr] Updated (%s): %d Items Queued, %d Retrieved",
			server.URL, queue.TotalRecords, len(queue.Records))
	}
}

// checkWhisparrQueue saves completed Whisparr-queued downloads to u.Map.
func (u *Unpackerr) checkWhisparrQueue(now time.Time) {
	for _, server := range u.Whisparr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Whisparr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Map[record.Title] = &Extract{
					App:         starr.Whisparr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Path:        u.getDownloadPath(record.OutputPath, starr.Whisparr, record.Title, server.Paths),
					IDs: map[string]any{
						"downloadId": record.DownloadID,
						"title":      record.Title,
						"movieId":    record.MovieID,
						"reason":     buildStatusReason(record.Status, record.StatusMessages),
					},
				}

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Whisparr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveWhisparrQitem(name string) bool {
	for _, server := range u.Whisparr {
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
