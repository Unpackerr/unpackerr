package unpackerr

import (
	"sync"

	"golift.io/cnfg"
	"golift.io/starr"
	"golift.io/starr/lidarr"
)

// LidarrConfig represents the input data for a Lidarr server.
type LidarrConfig struct {
	starr.Config
	Path           string        `json:"path" toml:"path" xml:"path" yaml:"path"`
	Paths          []string      `json:"paths" toml:"paths" xml:"paths" yaml:"paths"`
	Protocols      string        `json:"protocols" toml:"protocols" xml:"protocols" yaml:"protocols"`
	DeleteOrig     bool          `json:"delete_orig" toml:"delete_orig" xml:"delete_orig" yaml:"delete_orig"`
	DeleteDelay    cnfg.Duration `json:"delete_delay" toml:"delete_delay" xml:"delete_delay" yaml:"delete_delay"`
	Queue          *lidarr.Queue `json:"-" toml:"-" xml:"-" yaml:"-"`
	*lidarr.Lidarr `json:"-" toml:"-" xml:"-" yaml:"-"`
	sync.RWMutex   `json:"-" toml:"-" xml:"-" yaml:"-"`
}

func (u *Unpackerr) validateLidarr() {
	for i := range u.Lidarr {
		if u.Lidarr[i].Timeout.Duration == 0 {
			u.Lidarr[i].Timeout.Duration = u.Timeout.Duration
		}

		if u.Lidarr[i].DeleteDelay.Duration == 0 {
			u.Lidarr[i].DeleteDelay.Duration = u.DeleteDelay.Duration
		}

		if u.Lidarr[i].Path != "" {
			u.Lidarr[i].Paths = append(u.Lidarr[i].Paths, u.Lidarr[i].Path)
		}

		if len(u.Lidarr[i].Paths) == 0 {
			u.Lidarr[i].Paths = []string{defaultSavePath}
		}

		if u.Lidarr[i].Protocols == "" {
			u.Lidarr[i].Protocols = defaultProtocol
		}

		u.Lidarr[i].Lidarr = lidarr.New(&u.Lidarr[i].Config)
	}
}

func (u *Unpackerr) logLidarr() {
	if c := len(u.Lidarr); c == 1 {
		u.Printf(" => Lidarr Config: 1 server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, paths:%q",
			u.Lidarr[0].URL, u.Lidarr[0].APIKey != "", u.Lidarr[0].Timeout,
			u.Lidarr[0].ValidSSL, u.Lidarr[0].Protocols, u.Lidarr[0].Paths)
	} else {
		u.Print(" => Lidarr Config:", c, "servers")

		for _, f := range u.Lidarr {
			u.Printf(" =>    Server: %s, apikey:%v, timeout:%v, verify ssl:%v, protos:%s, paths:%q",
				f.URL, f.APIKey != "", f.Timeout, f.ValidSSL, f.Protocols, f.Paths)
		}
	}
}

// getLidarrQueue saves the Lidarr Queue(s).
func (u *Unpackerr) getLidarrQueue() {
	for i, server := range u.Lidarr {
		if server.APIKey == "" {
			u.Debugf("Lidarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.GetQueue(DefaultQueuePageSize)
		if err != nil {
			u.Printf("[ERROR] Lidarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		u.Lidarr[i].Queue = queue

		u.Printf("[Lidarr] Updated (%s): %d Items Queued, %d Retreived",
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
				u.Debugf("%s (%s): Item Waiting for Import (%s): %v", Lidarr, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && u.isComplete(q.Status, q.Protocol, server.Protocols):
				// This shoehorns the Lidarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					&starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, &Extract{
					App:         Lidarr,
					DeleteOrig:  server.DeleteOrig,
					DeleteDelay: server.DeleteDelay.Duration,
					Path:        u.getDownloadPath(q.StatusMessages, Lidarr, q.Title, server.Paths),
					IDs:         map[string]interface{}{"artistId": q.ArtistID, "albumId": q.AlbumID, "downloadId": q.DownloadID},
				})

				fallthrough
			default:
				u.Debugf("%s: (%s): %s (%s:%d%%): %v",
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
