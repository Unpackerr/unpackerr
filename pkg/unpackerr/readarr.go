package unpackerr

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golift.io/starr"
	"golift.io/starr/readarr"
)

// ReadarrConfig represents the input data for a Readarr server.
type ReadarrConfig struct {
	starr.Config
	StarrConfig
	Queue            *readarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex     `json:"-" toml:"-" xml:"-" yaml:"-"`
	*readarr.Readarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateReadarr() error {
	tmp := u.Readarr[:0]

	for i := range u.Readarr {
		if u.Readarr[i].URL == "" {
			u.Errorf("Missing Readarr URL in one of your configurations, skipped and ignored.")
			continue
		}

		if !strings.HasPrefix(u.Readarr[i].URL, "http://") && !strings.HasPrefix(u.Readarr[i].URL, "https://") {
			return fmt.Errorf("%w: (readarr) %s", ErrInvalidURL, u.Readarr[i].URL)
		}

		if len(u.Readarr[i].APIKey) != apiKeyLength {
			return fmt.Errorf("%s (%s) %w, your key length: %d",
				starr.Readarr, u.Readarr[i].URL, ErrInvalidKey, len(u.Readarr[i].APIKey))
		}

		if u.Readarr[i].Timeout.Duration == 0 {
			u.Readarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Readarr[i].DeleteDelay.Duration == 0 {
			u.Readarr[i].DeleteDelay.Duration = u.DeleteDelay.Duration
		}

		if u.Readarr[i].Path != "" {
			u.Readarr[i].Paths = append(u.Readarr[i].Paths, u.Readarr[i].Path)
		}

		if len(u.Readarr[i].Paths) == 0 {
			u.Readarr[i].Paths = []string{defaultSavePath}
		}

		if u.Readarr[i].Protocols == "" {
			u.Readarr[i].Protocols = defaultProtocol
		}

		u.Readarr[i].Config.Client = &http.Client{
			Timeout: u.Readarr[i].Timeout.Duration,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: !u.Readarr[i].ValidSSL}, //nolint:gosec
			},
		}

		u.Readarr[i].Readarr = readarr.New(&u.Readarr[i].Config)
		tmp = append(tmp, u.Readarr[i])
	}

	u.Readarr = tmp

	return nil
}

func (u *Unpackerr) logReadarr() {
	if c := len(u.Readarr); c == 1 {
		u.Printf(" => Readarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
			"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
			u.Readarr[0].URL, u.Readarr[0].APIKey != "", u.Readarr[0].Timeout,
			u.Readarr[0].ValidSSL, u.Readarr[0].Protocols, u.Readarr[0].Syncthing,
			u.Readarr[0].DeleteOrig, u.Readarr[0].DeleteDelay.Duration, u.Readarr[0].Paths)
	} else {
		u.Printf(" => Readarr Config: %d servers", c)

		for _, f := range u.Readarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
				"syncthing: %v, delete_orig: %v, delete_delay: %v, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.Syncthing, f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getReadarrQueue saves the Readarr Queue(s).
func (u *Unpackerr) getReadarrQueue() {
	for _, server := range u.Readarr {
		if server.APIKey == "" {
			u.Debugf("Readarr (%s): skipped, no API key", server.URL)
			continue
		}

		start := time.Now()

		queue, err := server.GetQueue(DefaultQueuePageSize, DefaultQueuePageSize)
		if err != nil {
			u.Errorf("Readarr (%s): %v", server.URL, err)
			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.saveQueueMetrics(server.Queue.TotalRecords, start, starr.Readarr, server.URL)

		if !u.Activity || queue.TotalRecords > 0 {
			u.Printf("[Readarr] Updated (%s): %d Items Queued, %d Retrieved", server.URL, queue.TotalRecords, len(queue.Records))
		}
	}
}

// checkReadarQueue passes completed Readar-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkReadarrQueue() {
	for _, server := range u.Readarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Readarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Readar OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					&starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})

				u.handleCompletedDownload(q.Title, &Extract{
					App:         starr.Readarr,
					URL:         server.URL,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Syncthing:   server.Syncthing,
					Path:        u.getDownloadPath(q.StatusMessages, starr.Readarr, q.Title, server.Paths),
					IDs: map[string]interface{}{
						"title":      q.Title,
						"authorId":   q.AuthorID,
						"bookId":     q.BookID,
						"downloadId": q.DownloadID,
					},
				})

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Readarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveReadarrQitem(name string) bool {
	for _, server := range u.Readarr {
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
