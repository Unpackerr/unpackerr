package unpacker

import (
	"fmt"
	"sync"

	"golift.io/starr"
)

// RadarrConfig represents the input data for a Radarr server.
type RadarrConfig struct {
	*starr.Config
	Path         string              `json:"path" toml:"path" xml:"path" yaml:"path"`
	Protocols    string              `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	Queue        []*starr.RadarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateRadarr() {
	for i := range u.Radarr {
		if u.Radarr[i].Timeout.Duration == 0 {
			u.Radarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Radarr[i].Path == "" {
			u.Radarr[i].Path = defaultSavePath
		}

		if u.Radarr[i].Protocols == "" {
			u.Radarr[i].Protocols = defaultProtocol
		}
	}
}

func (u *Unpackerr) logRadarr() {
	if c := len(u.Radarr); c == 1 {
		u.Logf(" => Radarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
			u.Radarr[0].URL, u.Radarr[0].Path, u.Radarr[0].APIKey != "",
			u.Radarr[0].Timeout, u.Radarr[0].ValidSSL, u.Radarr[0].Protocols)
	} else {
		u.Log(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
				f.URL, f.Path, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols)
		}
	}
}

// getRadarrQueue saves the Radarr Queue(s).
func (u *Unpackerr) getRadarrQueue() {
	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.Debug("Radarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.RadarrQueue()
		if err != nil {
			u.Logf("[ERROR] Radarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Logf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// checkRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkRadarrQueue() {
	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", Radarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.handleCompletedDownload(q.Title, Radarr, u.getDownloadPath(q.StatusMessages, Radarr, q.Title, server.Path),
					fmt.Sprintf("tmdbId:%d", q.Movie.TmdbID), fmt.Sprintf("imdbId:%s", q.Movie.ImdbID))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					Radarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveRadarrQitem(name string) bool {
	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
