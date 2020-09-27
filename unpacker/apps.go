package unpacker

import (
	"os"
	"path/filepath"
	"strings"

	"golift.io/starr"
)

// LidarrQueuePageSize is how many items we request from Lidarr.
// If you have more than this many items queued.. oof.
// As the queue goes away, more things should get picked up.
const LidarrQueuePageSize = 2000

// prefixPathMsg is used to locate/parse a download's path from a text string in StatusMessages.
const prefixPathMsg = "No files found are eligible for import in " // confirmed on Sonarr.

// getLidarrQueue saves the Lidarr Queue(s)
func (u *Unpackerr) getLidarrQueue() {
	for _, server := range u.Lidarr {
		if server.APIKey == "" {
			u.Debug("Lidarr (%s): skipped, no API key", server.URL)
			continue
		}

		queue, err := server.LidarrQueue(LidarrQueuePageSize)
		if err != nil {
			u.Logf("[ERROR] Lidarr (%s): %v", server.URL, err)
			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		u.Logf("[Lidarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Sonarr Queue(s)
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

// getSonarrQueue saves the Radarr Queue(s)
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
				u.Debug("%s (%s): %s (%s:%d%%): %v (Ep: %v, not complete, or not a torrent)",
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
				u.Debug("%s: (%s): %s (%s:%d%%): %v (not complete, or not a torrent)",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checkLidarrQueue passes completed Lidarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkLidarrQueue() {
	app := "Lidarr"

	for _, server := range u.Lidarr {
		for _, q := range server.Queue {
			switch x, ok := u.Map[q.Title]; {
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				// This shoehorns the Lidarr OutputPath into a StatusMessage that getDownloadPath can parse.
				q.StatusMessages = append(q.StatusMessages, starr.StatusMessage{Title: q.Title, Messages: []string{prefixPathMsg + q.OutputPath}})
				u.handleCompletedDownload(q.Title, app, u.getDownloadPath(q.StatusMessages, app, q.Title, server.Path))
				fallthrough
			default:
				u.Debug("%s: (%s): %s (%s:%d%%): %v (not complete, or not a torrent)",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checks if the application currently has an item in its queue
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

// checks if the application currently has an item in its queue
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

// checks if the application currently has an item in its queue
func (u *Unpackerr) haveLidarrQitem(name string) bool {
	for _, server := range u.Lidarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return true
			}
		}
	}

	return false
}

// Looking for a message that looks like:
// "No files found are eligible for import in /downloads/Downloading/Space.Warriors.S99E88.GrOuP.1080p.WEB.x264"
func (u *Unpackerr) getDownloadPath(s []starr.StatusMessage, app, title, path string) string {
	path = filepath.Join(path, title)
	if _, err := os.Stat(path); err == nil {
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
				u.Debug("%s: Configured path (%s) does not exist; trying path found in error message: %s", app, path, newPath)

				return newPath
			}
		}
	}

	u.Debug("%s: Configured path does not exist, and could not find alternative path in error message: %s ", app, path)

	return path
}
