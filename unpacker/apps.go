package unpacker

import (
	"path/filepath"

	"golift.io/starr"
)

// LidarrQueuePageSize is how many items we request from Lidarr.
// If you have more than this many items queued.. oof.
const LidarrQueuePageSize = 2000

// getLidarrQueue saves the Lidarr Queue(s)
func (u *Unpackerr) getLidarrQueue() {
	for i, server := range u.Lidarr {
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
		u.Lidarr[i].Queue = queue
		u.Logf("[Lidarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Sonarr Queue(s)
func (u *Unpackerr) getSonarrQueue() {
	for i, server := range u.Sonarr {
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
		u.Sonarr[i].Queue = queue
		u.Logf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Radarr Queue(s)
func (u *Unpackerr) getRadarrQueue() {
	for i, server := range u.Radarr {
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
		u.Radarr[i].Queue = queue
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
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				u.handleCompletedDownload(q.Title, app, filepath.Join(server.Path, q.Title))
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import: %v", app, server.URL, q.Title)
			case !ok:
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
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				u.handleCompletedDownload(q.Title, app, filepath.Join(server.Path, q.Title))
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case !ok:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
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
			case (!ok || x.Status < QUEUED) && q.Status == completed && q.Protocol == torrent:
				u.handleCompletedDownload(q.Title, app, q.OutputPath)
			case ok && x.Status == EXTRACTED && q.Status == completed && q.Protocol == torrent:
				u.Debug("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case !ok:
				u.Debug("%s: (%s): %s (%s:%d%%): %v",
					app, server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// gets a sonarr queue item based on name. returns first match
func (u *Unpackerr) getSonarQitem(name string) *starr.SonarQueue {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return q
			}
		}
	}

	return nil
}

// gets a radarr queue item based on name. returns first match
func (u *Unpackerr) getRadarQitem(name string) *starr.RadarQueue {
	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return q
			}
		}
	}

	return nil
}

// gets a lidarr queue item based on name. returns first match
func (u *Unpackerr) getLidarQitem(name string) *starr.LidarrRecord {
	for _, server := range u.Lidarr {
		for _, q := range server.Queue {
			if q.Title == name {
				return q
			}
		}
	}

	return nil
}
