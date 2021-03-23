package unpackerr

import (
	"fmt"
	"strings"
	"sync"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/starr/radarr"
)

// RadarrConfig represents the input data for a Radarr server.
type RadarrConfig struct {
	starr.Config
	Path           string          `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths          []string        `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols      string          `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	DeleteOrig     bool            `json:"delete_orig" toml:"delete_orig" xml:"delete_orig" yaml:"delete_orig"`
	DeleteDelay    cnfg.Duration   `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Queue          []*radarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
	*radarr.Radarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateRadarr() error {
	for i := range u.Radarr {
		if !strings.HasPrefix(u.Radarr[i].URL, "http") {
			return fmt.Errorf("%w: %s", ErrInvalidURL, u.Radarr[i].URL)
		}

		if len(u.Radarr[i].APIKey) < 5 {
			return fmt.Errorf("%w: %s", ErrInvalidKey, u.Radarr[i].APIKey)
		}

		if u.Radarr[i].Timeout.Duration == 0 {
			u.Radarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Radarr[i].DeleteDelay.Duration == 0 {
			u.Radarr[i].DeleteDelay.Duration = u.DeleteDelay.Duration
		}

		if u.Radarr[i].Path != "" {
			u.Radarr[i].Paths = append(u.Radarr[i].Paths, u.Radarr[i].Path)
		}

		if len(u.Radarr[i].Paths) == 0 {
			u.Radarr[i].Paths = []string{defaultSavePath}
		}

		if u.Radarr[i].Protocols == "" {
			u.Radarr[i].Protocols = defaultProtocol
		}

		u.Radarr[i].Radarr = radarr.New(&u.Radarr[i].Config)
	}

	return nil
}

func (u *Unpackerr) logRadarr() {
	if c := len(u.Radarr); c == 1 {
		u.Printf(" => Radarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
			"delete_orig: %v, delete_delay: %v, paths:%q",
			u.Radarr[0].URL, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout,
			u.Radarr[0].ValidSSL, u.Radarr[0].Protocols, u.Radarr[0].DeleteOrig,
			u.Radarr[0].DeleteDelay.Duration, u.Radarr[0].Paths)
	} else {
		u.Print(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
				"delete_orig: %v, delete_delay: %v, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getRadarrQueue saves the Radarr Queue(s).
func (u *Unpackerr) getRadarrQueue() {
	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.Debugf("Radarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.GetQueueV2()
		if err != nil {
			u.Printf("[ERROR] Radarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Printf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// checkRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkRadarrQueue() {
	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", Radarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				x := &Extract{
					App:         Radarr,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Path:        u.getDownloadPath(q.StatusMessages, Radarr, q.Title, server.Paths),
					IDs:         map[string]interface{}{"downloadId": q.DownloadID, "title": q.Title},
				}

				if q.Movie != nil {
					x.IDs["title"] = q.Movie.Title
					x.IDs["tmdbId"] = q.Movie.TmdbID
					x.IDs["imdbId"] = q.Movie.ImdbID
				}

				u.handleCompletedDownload(q.Title, x)

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
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
