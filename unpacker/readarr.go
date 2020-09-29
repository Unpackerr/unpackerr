package unpacker

import (
	"sync"

	"golift.io/starr"
)

// ReadarrConfig represents the input data for a Readarr server.
type ReadarrConfig struct {
	*starr.Config
	Path         string             `json:"path" toml:"path" xml:"path" yaml:"path"`
	Protocols    string             `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	Queue        *starr.ReadarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateReadarr() {
	for i := range u.Readarr {
		if u.Readarr[i].Timeout.Duration == 0 {
			u.Readarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Readarr[i].Path == "" {
			u.Readarr[i].Path = defaultSavePath
		}

		if u.Readarr[i].Protocols == "" {
			u.Readarr[i].Protocols = defaultProtocol
		}
	}
}

func (u *Unpackerr) logReadarr() {
	if c := len(u.Readarr); c == 1 {
		u.Logf(" => Readarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
			u.Readarr[0].URL, u.Readarr[0].Path, u.Readarr[0].APIKey != "",
			u.Readarr[0].Timeout, u.Readarr[0].ValidSSL, u.Readarr[0].Protocols)
	} else {
		u.Log(" => Readarr Config:", c, "servers")

		for _, f := range u.Readarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
				f.URL, f.Path, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols)
		}
	}
}

// getReadarrQueue saves the Readarr Queue(s).
func (u *Unpackerr) getReadarrQueue() {
	for i, server := range u.Readarr {
		if server.APIKey == "" {
			u.Debug("Readarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.ReadarrQueue(DefaultQueuePageSize)
		if err != nil {
			u.Logf("[ERROR] Readarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		u.Readarr[i].Queue = queue
		u.Logf("[Readarr] Updated (%s): %d Items Queued, %d Retreived",
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
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", Readarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Readar OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, Readarr, u.getDownloadPath(q.StatusMessages, Readarr, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
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
