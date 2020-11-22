package unpacker

import (
	"fmt"
	"sync"

	"golift.io/starr"
)

// SonarrConfig represents the input data for a Sonarr server.
type SonarrConfig struct {
	*starr.Config
	Path         string              `json:"path" toml:"path" xml:"path" yaml:"path"`
	Protocols    string              `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	Queue        []*starr.SonarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateSonarr() {
	for i := range u.Sonarr {
		if u.Sonarr[i].Timeout.Duration == 0 {
			u.Sonarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Sonarr[i].Path == "" {
			u.Sonarr[i].Path = defaultSavePath
		}

		if u.Sonarr[i].Protocols == "" {
			u.Sonarr[i].Protocols = defaultProtocol
		}
	}
}

func (u *Unpackerr) logSonarr() {
	if c := len(u.Sonarr); c == 1 {
		u.Logf(" => Sonarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
			u.Sonarr[0].URL, u.Sonarr[0].Path, u.Sonarr[0].APIKey != "",
			u.Sonarr[0].Timeout, u.Sonarr[0].ValidSSL, u.Sonarr[0].Protocols)
	} else {
		u.Log(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
				f.URL, f.Path, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols)
		}
	}
}

// getSonarrQueue saves the Sonarr Queue(s).
func (u *Unpackerr) getSonarrQueue() {
	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.Debug("Sonarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.SonarrQueue()
		if err != nil {
			u.Logf("[ERROR] Sonarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Logf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debug("%s (%s): Item Waiting for Import: %v", Sonarr, server.URL, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.handleCompletedDownload(q.Title, Sonarr, u.getDownloadPath(q.StatusMessages, Sonarr, q.Title, server.Path),
					fmt.Sprintf("tvdbId:%d", q.Series.TvdbID), fmt.Sprintf("imdbId:%s", q.Series.ImdbID),
					fmt.Sprintf("seriesId:%d", q.Episode.SeriesID), fmt.Sprintf("downloadId:%s", q.DownloadID),
					fmt.Sprintf("tvRageId:%d", q.Series.TvRageID), fmt.Sprintf("tvMazeId:%d", q.Series.TvMazeID))

				fallthrough
			default:
				u.Debug("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
					Sonarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title, q.Episode.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveSonarrQitem(name string) bool {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
