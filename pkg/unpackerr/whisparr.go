package unpackerr

import (
	"time"

	"golift.io/starr"
)

// WhisparrConfig just uses radarr. Queue poll/have is shared via starrApp on *RadarrConfig.
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

// checkWhisparrQueue saves completed Whisparr-queued downloads to u.Map.
func (u *Unpackerr) checkWhisparrQueue(now time.Time) {
	u.lockHistory()
	defer u.unlockHistory()

	for _, server := range u.Whisparr {
		if server.Queue == nil {
			continue
		}

		for _, record := range server.Queue.Records {
			switch x, ok := u.Map[record.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(record.Status, record.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Whisparr, server.URL, record.Protocol, record.Title)
			case !ok && u.isComplete(record.Status, record.Protocol, server.Protocols) && !u.isForgotten(record.Title):
				u.Map[record.Title] = &Extract{
					App:         starr.Whisparr,
					URL:         server.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					MaxBytes:    server.maxBytes,
					Path:        u.getDownloadPath(record.OutputPath, starr.Whisparr, record.Title, server.Paths),
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
					starr.Whisparr, server.URL, record.Status, record.Protocol,
					percent(record.Sizeleft, record.Size), record.Title)
			}
		}
	}
}
