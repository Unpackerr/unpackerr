package unpackerr

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golift.io/starr"
	"golift.io/starr/sonarr"
)

// SonarrConfig represents the input data for a Sonarr server.
type SonarrConfig struct {
	starr.Config
	StarrConfig
	Queue          *sonarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
	*sonarr.Sonarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateSonarr() error {
	tmp := u.Sonarr[:0]

	for i := range u.Sonarr {
		if u.Sonarr[i].URL == "" {
			u.Printf("Missing Sonarr URL in one of your configurations, skipped and ignored.")
			continue
		}

		if !strings.HasPrefix(u.Sonarr[i].URL, "http://") && !strings.HasPrefix(u.Sonarr[i].URL, "https://") {
			return fmt.Errorf("%w: (sonarr) %s", ErrInvalidURL, u.Sonarr[i].URL)
		}

		if len(u.Sonarr[i].APIKey) != apiKeyLength {
			return fmt.Errorf("%s (%s) %w, your key length: %d",
				starr.Sonarr, u.Sonarr[i].URL, ErrInvalidKey, len(u.Sonarr[i].APIKey))
		}

		if u.Sonarr[i].Timeout.Duration == 0 {
			u.Sonarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Sonarr[i].DeleteDelay.Duration == 0 {
			u.Sonarr[i].DeleteDelay.Duration = u.DeleteDelay.Duration
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

		u.Sonarr[i].Config.Client = &http.Client{
			Timeout: u.Sonarr[i].Timeout.Duration,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: !u.Sonarr[i].ValidSSL},
			},
		}

		u.Sonarr[i].Sonarr = sonarr.New(&u.Sonarr[i].Config)
		tmp = append(tmp, u.Sonarr[i])
	}

	u.Sonarr = tmp

	return nil
}

func (u *Unpackerr) logSonarr() {
	if c := len(u.Sonarr); c == 1 {
		u.Printf(" => Sonarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
			"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
			u.Sonarr[0].URL, u.Sonarr[0].APIKey != "", u.Sonarr[0].Timeout,
			u.Sonarr[0].ValidSSL, u.Sonarr[0].Protocols, u.Sonarr[0].Syncthing,
			u.Sonarr[0].DeleteOrig, u.Sonarr[0].DeleteDelay.Duration, u.Sonarr[0].Paths)
	} else {
		u.Print(" => Sonarr Config:", c, "servers")

		for _, f := range u.Sonarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
				"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
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

		queue, err := server.GetQueue(DefaultQueuePageSize, 1)
		if err != nil {
			u.Printf("[ERROR] Sonarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue

		if !u.Activity || queue.TotalRecords > 0 {
			u.Printf("[Sonarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
		}
	}
}

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	for _, server := range u.Sonarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import: %v", Sonarr, server.URL, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Sonarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					&starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})

				u.handleCompletedDownload(q.Title, &Extract{
					App:         Sonarr,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(q.StatusMessages, Sonarr, q.Title, server.Paths),
					IDs: map[string]interface{}{
						"title":      q.Title,
						"downloadId": q.DownloadID,
						"seriesId":   q.SeriesID,
						"episodeId":  q.EpisodeID,
					},
				})

				fallthrough
			default:
				u.Debugf("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
					Sonarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title, q.EpisodeID)
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

		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
