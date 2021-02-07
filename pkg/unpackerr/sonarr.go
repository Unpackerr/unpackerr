package unpackerr

import (
	"sync"

	"golift.io/starr"
	"golift.io/starr/sonarr"
)

// SonarrConfig represents the input data for a Sonarr server.
type SonarrConfig struct {
	*starr.Config
	Path           string            `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths          []string          `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols      string            `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	Queue          []*sonarr.QueueV2 `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
	*sonarr.Sonarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateSonarr() {
	for i := range u.Sonarr {
		if u.Sonarr[i].Timeout.Duration == 0 {
			u.Sonarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Sonarr[i].Path != "" {
			u.Sonarr[i].Paths = append(u.Sonarr[i].Paths, u.Sonarr[i].Path)
		}

		if len(u.Sonarr[i].Paths) == 0 {
			u.Sonarr[i].Paths = []string{defaultSavePath}
		}

		if u.Sonarr[i].Protocols == "" {
			u.Sonarr[i].Protocols = defaultProtocol
		}

		u.Sonarr[i].Sonarr = sonarr.New(u.Sonarr[i].Config)
	}
}

func (u *Unpackerr) logSonarr() {
	if c := len(u.Sonarr); c == 1 {
		u.Printf(" => Sonarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, paths:%q",
			u.Sonarr[0].URL, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout,
			u.Sonarr[0].ValidSSL, u.Sonarr[0].Protocols, u.Sonarr[0].Paths)
	} else {
		u.Print(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols, f.Paths)
		}
	}
}

// getSonarrQueue saves the Sonarr Queue(s).
func (u *Unpackerr) getSonarrQueue() {
	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.Debugf("Sonarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.GetQueueV2()
		if err != nil {
			u.Printf("[ERROR] Sonarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Printf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import: %v", Sonarr, server.URL, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.handleCompletedDownload(q.Title, Sonarr, u.getDownloadPath(q.StatusMessages, Sonarr, q.Title, server.Paths),
					map[string]interface{}{
						"tvdbId": q.Series.TvdbID, "imdbId": q.Series.ImdbID, "downloadId": q.DownloadID,
						"seriesId": q.Episode.SeriesID, "tvRageId": q.Series.TvRageID, "tvMazeId": q.Series.TvMazeID,
					})

				fallthrough
			default:
				u.Debugf("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
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
