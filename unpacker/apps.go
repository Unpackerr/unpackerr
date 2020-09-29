package unpacker

import (
	"os"
	"path/filepath"
	"strings"

	"golift.io/starr"
)

// DefaultQueuePageSize is how many items we request from Lidarr and Readarr.
// Once we have better support for Sonarr/Radarr v3 this will apply to those as well.
// If you have more than this many items queued.. oof.
// As the queue goes away, more things should get picked up.
const DefaultQueuePageSize = 2000

// prefixPathMsg is used to locate/parse a download's path from a text string in StatusMessages.
const prefixPathMsg = "No files found are eligible for import in " // confirmed on Sonarr.

// getReadarrQueue saves the Readarr Queue(s).
func (u *Unpackerr) getReadarrQueue() {
	for _, server := range u.Readarr {
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
		server.Queue = queue
		u.Logf("[Readarr] Updated (%s): %d Items Grabbed, %d Queued", server.URL, len(queue.Records), queue.TotalRecords)
	}
}

// getLidarrQueue saves the Lidarr Queue(s).
func (u *Unpackerr) getLidarrQueue() {
	for _, server := range u.Lidarr {
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
		server.Queue = queue
		u.Logf("[Lidarr] Updated (%s): %d Items Grabbed, %d Queued", server.URL, len(queue.Records), queue.TotalRecords)
	}
}

// getSonarrQueue saves the Sonarr Queue(s).
func (u *Unpackerr) getSonarrQueue() {
	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.Debug("Sonarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.SonarrQueue()
		if err != nil {
			u.Logf("[ERROR] Sonarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Logf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Radarr Queue(s).
func (u *Unpackerr) getRadarrQueue() {
	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.Debug("Radarr (%s): skipped, no API key", server.URL)

			continue
		}

		queue, err := server.RadarrQueue()
		if err != nil {
			u.Logf("[ERROR] Radarr (%s): %v", server.URL, err)

			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Logf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// custom percentage procedure for *arr apps.
func percent(size, total float64) int {
	const oneHundred = 100

	return int(oneHundred - (size / total * oneHundred))
}

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	app := "Sonarr"

	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import: %v", app, server.URL, q.Title)
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				u.handleCompletedDownload(q.Title, app, u.getDownloadPath(q.StatusMessages, app, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title, q.Episode.Title)
			}
		}
	}
}

// checkRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkRadarrQueue() {
	app := "Radarr"

	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				u.handleCompletedDownload(q.Title, app, u.getDownloadPath(q.StatusMessages, app, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checkLidarrQueue passes completed Lidarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkLidarrQueue() { // nolint: dupl
	app := "Lidarr"

	for _, server := range u.Lidarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				// This shoehorns the Lidarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, app, u.getDownloadPath(q.StatusMessages, app, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checkReadarQueue passes completed Readar-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkReadarrQueue() { // nolint: dupl
	app := "Readar"

	for _, server := range u.Readarr {
		if server.Queue == nil {
			continue
		}

		for _, q := range server.Queue.Records {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				// This shoehorns the Readar OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages,
					starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, app, u.getDownloadPath(q.StatusMessages, app, q.Title, server.Path))

				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
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

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveLidarrQitem(name string) bool {
	for _, server := range u.Lidarr {
		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}

// checks if the application currently has an item in its queue.
func (u *Unpackerr) haveReadarrQitem(name string) bool {
	for _, server := range u.Readarr {
		for _, q := range server.Queue.Records {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}

// Looking for a message that looks like:
// "No files found are eligible for import in /downloads/Downloading/Space.Warriors.S99E88.GrOuP.1080p.WEB.x264".
func (u *Unpackerr) getDownloadPath(s []starr.StatusMessage, app, title, path string) string {
	var err error

	path = filepath.Join(path, title)
	if _, err = os.Stat(path); err == nil {
		u.Debug("%s: Configured path exists: %s", app, path)

		return path // the server path exists, so use that.
	}

	for _, m := range s {
		if m.Title != title {
			continue
		}

		for _, msg := range m.Messages {
			if strings.HasPrefix(msg, prefixPathMsg) && strings.HasSuffix(msg, title) {
				newPath := strings.TrimSpace(strings.TrimPrefix(msg, prefixPathMsg))
				u.Debug("%s: Configured path (%s, err: %v) does not exist; trying path found in status message: %s",
					app, path, err, newPath)

				return newPath
			}
		}
	}

	u.Debug("%s: Configured path does not exist (err: %v), and could not find alternative path in error message: %s ",
		app, err, path)

	return path
}
