package unpacker

import (
	"log"
	"path/filepath"

	"golift.io/starr"
)

// LidarrQueuePageSize is how many items we request from Lidarr.
// If you have more than this many items queued.. oof.
const LidarrQueuePageSize = 2000

// getLidarrQueue saves the Lidarr Queue(s)
func (u *Unpackerr) getLidarrQueue() {
	for _, server := range u.Lidarr {
		if server.APIKey == "" {
			u.DeLogf("Lidarr (%s): skipped, no API key", server.URL)
			continue
		}

		queue, err := server.LidarrQueue(LidarrQueuePageSize)
		if err != nil {
			log.Printf("[ERROR] Lidarr (%s): %v", server.URL, err)
			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		log.Printf("[Lidarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Sonarr Queue(s)
func (u *Unpackerr) getSonarrQueue() {
	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.DeLogf("Sonarr (%s): skipped, no API key", server.URL)
			continue
		}

		queue, err := server.SonarrQueue()
		if err != nil {
			log.Printf("[ERROR] Sonarr (%s): %v", server.URL, err)
			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		log.Printf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
}

// getSonarrQueue saves the Radarr Queue(s)
func (u *Unpackerr) getRadarrQueue() {
	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.DeLogf("Radarr (%s): skipped, no API key", server.URL)
			continue
		}

		queue, err := server.RadarrQueue()
		if err != nil {
			log.Printf("[ERROR] Radarr (%s): %v", server.URL, err)
			return
		}

		// Only update if there was not an error fetching.
		server.Queue = queue
		log.Printf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(queue))
	}
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
				u.DeLogf("%s (%s): Item Waiting for Import: %v", app, server.URL, q.Title)
			case !ok:
				u.DeLogf("%s (%s): %s (%s:%d%%): %v (Ep: %v)",
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
				u.DeLogf("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case !ok:
				u.DeLogf("%s: (%s): %s (%s:%d%%): %v",
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
				u.DeLogf("%s (%s): Item Waiting for Import (%s): %v", app, server.URL, q.Protocol, q.Title)
			case !ok:
				u.DeLogf("%s: (%s): %s (%s:%d%%): %v",
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
