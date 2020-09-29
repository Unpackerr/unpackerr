package unpacker

import (
	"sync"

	"golift.io/starr"
)

// LidarConfig represents the input data for a Lidarr server.
type LidarrConfig struct {
	*starr.Config
	Path         string            `json:"path" toml:"path" xml:"path" yaml:"path"`
	Protocols    string            `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	Queue        *starr.LidarQueue `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateLidarr() {
	for i := range u.Lidarr {
		if u.Lidarr[i].Timeout.Duration == 0 {
			u.Lidarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Lidarr[i].Path == "" {
			u.Lidarr[i].Path = defaultSavePath
		}

		if u.Lidarr[i].Protocols == "" {
			u.Lidarr[i].Protocols = defaultProtocol
		}
	}
}

func (u *Unpackerr) logLidarr() {
	if c := len(u.Lidarr); c == 1 {
		u.Logf(" => Lidarr Config: 1 server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
			u.Lidarr[0].URL, u.Lidarr[0].Path, u.Lidarr[0].APIKey != "",
			u.Lidarr[0].Timeout, u.Lidarr[0].ValidSSL, u.Lidarr[0].Protocols)
	} else {
		u.Log(" => Lidarr Config:", c, "servers")

		for _, f := range u.Lidarr {
			u.Logf(" =>    Server: %s @ %s (apikey: %v, timeout: %v, verify ssl: %v, protos:%s)",
				f.URL, f.Path, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols)
		}
	}
}

// getLidarrQueue saves the Lidarr Queue(s).
func (u *Unpackerr) getLidarrQueue() {
	for i, server := range u.Lidarr {
		if server.APIKey == "" {
			u.Debug("Lidarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.LidarrQueue(DefaultQueuePageSize)
		if err != nil {
			u.Logf("[ERROR] Lidarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		u.Lidarr[i].Queue = queue

		u.Logf("[Lidarr] Updated (%s): %d Items Queued, %d Retreived",
			server.URL, queue.TotalRecords, len(u.Lidarr[i].Queue.Records))
	}
}

// checkLidarrQueue passes completed Lidarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkLidarrQueue() {
	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && u.isComplete(q.Status, q.Protocol, server.Protocols):
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", Lidarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Lidarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, Lidarr, u.getDownloadPath(q.StatusMessages, Lidarr, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					Lidarr, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveLidarrQitem(name string) bool {
	for _, server := range u.Lidarr {
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
