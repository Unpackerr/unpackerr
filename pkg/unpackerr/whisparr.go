package unpackerr

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
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

	for i := range u.Whisparr {
		if u.Whisparr[i].URL == "" {
			u.Errorf("Missing Whisparr URL in one of your configurations, skipped and ignored.")
			continue
		}

		if u.Whisparr[i].APIKey == "" {
			u.Errorf("Missing Whisparr API Key in one of your configurations, skipped and ignored.")
			continue
		}

		if !strings.HasPrefix(u.Whisparr[i].URL, "http://") && !strings.HasPrefix(u.Whisparr[i].URL, "https://") {
			return fmt.Errorf("%w: (whisparr) %s", ErrInvalidURL, u.Whisparr[i].URL)
		}

		if len(u.Whisparr[i].APIKey) != apiKeyLength {
			return fmt.Errorf("%s (%s) %w, your key length: %d",
				"Whisparr", u.Whisparr[i].URL, ErrInvalidKey, len(u.Whisparr[i].APIKey))
		}

		if u.Whisparr[i].Timeout.Duration == 0 {
			u.Whisparr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Whisparr[i].DeleteDelay.Duration == 0 {
			u.Whisparr[i].DeleteDelay.Duration = u.DeleteDelay.Duration
		}

		if u.Whisparr[i].Path != "" {
			u.Whisparr[i].Paths = append(u.Whisparr[i].Paths, u.Whisparr[i].Path)
		}

		if len(u.Whisparr[i].Paths) == 0 {
			u.Whisparr[i].Paths = []string{defaultSavePath}
		}

		if u.Whisparr[i].Protocols == "" {
			u.Whisparr[i].Protocols = defaultProtocol
		}

		u.Whisparr[i].Config.Client = &http.Client{
			Timeout: u.Whisparr[i].Timeout.Duration,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: !u.Whisparr[i].ValidSSL}, //nolint:gosec
			},
		}

		// shoehorned!
		u.Whisparr[i].Radarr = radarr.New(&u.Whisparr[i].Config)
		tmp = append(tmp, u.Whisparr[i])
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
func (u *Unpackerr) getWhisparrQueue() {
	for _, server := range u.Whisparr {
		if server.APIKey == "" {
			u.Debugf("Whisparr (%s): skipped, no API key", server.URL)
			continue
		}

		start := time.Now()

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
}

// checkWhisparrQueue passes completed Whisparr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkWhisparrQueue() {
	for _, server := range u.Whisparr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", starr.Whisparr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.saveCompletedDownload(q.Title, &Extract{
					App:         starr.Whisparr,
					URL:         server.URL,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Path:        u.getDownloadPath(q.OutputPath, starr.Whisparr, q.Title, server.Paths),
					IDs: map[string]interface{}{
						"downloadId": q.DownloadID,
						"title":      q.Title,
						"movieId":    q.MovieID,
						"reason":     buildStatusReason(q.Status, q.StatusMessages),
					},
				})

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
					starr.Whisparr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
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

		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}
