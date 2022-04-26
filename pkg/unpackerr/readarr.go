package unpackerr

import (
	"fmt"
	"strings"
	"sync"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/starr/readarr"
)

// ReadarrConfig represents the input data for a Readarr server.
type ReadarrConfig struct {
	starr.Config
	Path             string         `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths            []string       `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols        string         `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	DeleteOrig       bool           `json:"delete_orig" toml:"delete_orig" xml:"delete_orig" yaml:"delete_orig"`
	DeleteDelay      cnfg.Duration  `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Queue            *readarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex     `json:"-" toml:"-" xml:"-" yaml:"-"`
	*readarr.Readarr `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateReadarr() error {
	tmp := u.Readarr[:0]

	for i := range u.Readarr {
		if u.Readarr[i].URL == "" {
			u.Printf("Missing Readarr URL in one of your configurations, skipped and ignored.")
			continue
		}

		if !strings.HasPrefix(u.Readarr[i].URL, "http://") && !strings.HasPrefix(u.Readarr[i].URL, "https://") {
			return fmt.Errorf("%w: (readarr) %s", ErrInvalidURL, u.Readarr[i].URL)
		}

		if len(u.Readarr[i].APIKey) != apiKeyLength {
			u.Printf("Readarr (%s): ignored, invalid API key: %s", u.Readarr[i].URL, u.Readarr[i].APIKey)
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

		if r, err := u.Readarr[i].GetURL(); err != nil {
			u.Printf("[ERROR] Checking Readarr Path: %v", err)
		} else if r = strings.TrimRight(r, "/"); r != u.Readarr[i].URL {
			u.Printf("[WARN] Readarr URL fixed: %s -> %s (continuing)", u.Readarr[i].URL, r)
			u.Readarr[i].URL = r
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
			"delete_orig: %v, delete_delay: %v, paths:%q",
			u.Readarr[0].URL, u.Readarr[0].APIKey != "", u.Readarr[0].Timeout,
			u.Readarr[0].ValidSSL, u.Readarr[0].Protocols, u.Readarr[0].DeleteOrig,
			u.Readarr[0].DeleteDelay.Duration, u.Readarr[0].Paths)
	} else {
		u.Print(" => Readarr Config:", c, "servers")

		for _, f := range u.Readarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, "+
				"delete_orig: %v, delete_delay: %v, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols,
				f.DeleteOrig, f.DeleteDelay.Duration, f.Paths)
		}
	}
}

// getReadarrQueue saves the Readarr Queue(s).
func (u *Unpackerr) getReadarrQueue() {
	for i, server := range u.Readarr {
		if server.APIKey == "" {
			u.Debugf("Readarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.GetQueue(DefaultQueuePageSize, DefaultQueuePageSize)
		if err != nil {
			u.Printf("[ERROR] Readarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		u.Readarr[i].Queue = queue
		u.Printf("[Readarr] Updated (%s): %d Items Queued, %d Retrieved",
			server.URL, queue.TotalRecords, len(u.Readarr[i].Queue.Records))
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
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", Readarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Readar OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					&starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})

				u.handleCompletedDownload(q.Title, &Extract{
					App:         Readarr,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Path:        u.getDownloadPath(q.StatusMessages, Readarr, q.Title, server.Paths),
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
					Readarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
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
