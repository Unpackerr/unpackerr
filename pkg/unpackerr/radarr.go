package unpackerr

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"golift.io/starr"
	"golift.io/starr/radarr"
)

// RadarrConfig represents the input data for a Radarr server.
type RadarrConfig struct {
	starr.Config
	StarrConfig
	Queue          *radarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
	*radarr.Radarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateRadarr() error {
	tmp := u.Radarr[:0]

	for i := range u.Radarr {
		if u.Radarr[i].URL == "" {
			u.Printf("Missing Radarr URL in one of your configurations, skipped and ignored.")
			continue
		}

		if !strings.HasPrefix(u.Radarr[i].URL, "http://") && !strings.HasPrefix(u.Radarr[i].URL, "https://") {
			return fmt.Errorf("%w: (radarr) %s", ErrInvalidURL, u.Radarr[i].URL)
		}

		if len(u.Radarr[i].APIKey) != apiKeyLength {
			return fmt.Errorf("%s (%s) %w, your key length: %d",
				starr.Radarr, u.Radarr[i].URL, ErrInvalidKey, len(u.Radarr[i].APIKey))
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

		u.Radarr[i].Config.Client = &http.Client{
			Timeout: u.Radarr[i].Timeout.Duration,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: !u.Radarr[i].ValidSSL}, //nolint:gosec
			},
		}

		u.Radarr[i].Radarr = radarr.New(&u.Radarr[i].Config)
		tmp = append(tmp, u.Radarr[i])
	}

	u.Radarr = tmp

	return nil
}

func (u *Unpackerr) logRadarr() {
	if c := len(u.Radarr); c == 1 {
		u.Printf(" => Radarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
			"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
			u.Radarr[0].URL, u.Radarr[0].APIKey != "", u.Radarr[0].Timeout,
			u.Radarr[0].ValidSSL, u.Radarr[0].Protocols, u.Radarr[0].Syncthing,
			u.Radarr[0].DeleteOrig, u.Radarr[0].DeleteDelay.Duration, u.Radarr[0].Paths)
	} else {
		u.Print(" => Radarr Config:", c, "servers")

		for _, f := range u.Radarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
				"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
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

		queue, err := server.GetQueue(DefaultQueuePageSize, 1)
		if err != nil {
			u.Printf("[ERROR] Radarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue

		if !u.Activity || queue.TotalRecords > 0 {
			u.Printf("[Radarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
		}
	}
}

// checkRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkRadarrQueue() {
	for _, server := range u.Radarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", Radarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Radarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					&starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})

				u.handleCompletedDownload(q.Title, &Extract{
					App:         Radarr,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(q.StatusMessages, Radarr, q.Title, server.Paths),
					IDs: map[string]interface{}{
						"downloadId": q.DownloadID,
						"title":      q.Title,
						"movieId":    q.MovieID,
					},
				})

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
