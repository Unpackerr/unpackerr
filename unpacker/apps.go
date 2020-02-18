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
	var err error

	for _, server := range u.Lidarr {
		if server.APIKey == "" {
			u.DeLogf("Lidarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.LidarrQueue(LidarrQueuePageSize); err != nil {
			log.Printf("[ERROR] Lidarr (%s): %v", server.URL, err)
			return
		}

		log.Printf("[Lidarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// getSonarrQueue saves the Sonarr Queue(s)
func (u *Unpackerr) getSonarrQueue() {
	var err error

	for _, server := range u.Sonarr {
		if server.APIKey == "" {
			u.DeLogf("Sonarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.SonarrQueue(); err != nil {
			log.Printf("[ERROR] Sonarr (%s): %v", server.URL, err)
			return
		}

		log.Printf("[Sonarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// getSonarrQueue saves the Radarr Queue(s)
func (u *Unpackerr) getRadarrQueue() {
	var err error

	for _, server := range u.Radarr {
		if server.APIKey == "" {
			u.DeLogf("Radarr (%s): skipped, no API key", server.URL)
			continue
		}

		if server.Queue, err = server.RadarrQueue(); err != nil {
			log.Printf("[ERROR] Radarr (%s): %v", server.URL, err)
		}

		log.Printf("[Radarr] Updated (%s): %d Items Queued", server.URL, len(server.Queue))
	}
}

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			if _, ok := u.Map[q.Title]; !ok && q.Status == completed && q.Protocol == torrent {
				u.handleCompletedDownload(q.Title, "Sonarr", filepath.Join(server.Path, q.Title))
			} else if ok && q.Status == completed && q.Protocol == torrent {
				u.DeLogf("Sonarr (%s): Item Waiting For Import (%s): %v", server.URL, q.Protocol, q.Title)
			} else { // not done or not for us.
				u.DeLogf("Sonarr (%s): %s (%s:%d%%): %v (Ep: %v)",
					server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title, q.Episode.Title)
			}
		}
	}
}

// checkRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkRadarrQueue() {
	for _, server := range u.Radarr {
		for _, q := range server.Queue {
			if _, ok := u.Map[q.Title]; !ok && q.Status == completed && q.Protocol == torrent {
				u.handleCompletedDownload(q.Title, "Radarr", filepath.Join(server.Path, q.Title))
			} else if ok && q.Status == completed && q.Protocol == torrent {
				u.DeLogf("Radarr (%s): Item Waiting For Import (%s): %v", server.URL, q.Protocol, q.Title)
			} else { // not done or not for us.
				u.DeLogf("Radarr (%s): %s (%s:%d%%): %v",
					server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// checkLidarrQueue passes completed Lidarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkLidarrQueue() {
	for _, server := range u.Lidarr {
		for _, q := range server.Queue {
			if _, ok := u.Map[q.Title]; !ok && q.Status == completed && q.Protocol == torrent {
				u.handleCompletedDownload(q.Title, "Lidarr", q.OutputPath)
			} else if ok && q.Status == completed && q.Protocol == torrent {
				u.DeLogf("Lidarr (%s): Item Waiting For Import (%s): %v", server.URL, q.Protocol, q.Title)
			} else { // not done or not for us.
				u.DeLogf("Lidarr: (%s): %s (%s:%d%%): %v",
					server.URL, q.Status, q.Protocol, percent(q.Sizeleft, q.Size), q.Title)
			}
		}
	}
}

// gets a sonarr queue item based on name. returns first match
func (u *Unpackerr) getSonarQitem(name string) *starr.SonarQueue {
	for _, server := range u.Sonarr {
		for _, record := range server.Queue {
			if record.Title == name {
				return record
			}
		}
	}

	return nil
}

// gets a radarr queue item based on name. returns first match
func (u *Unpackerr) getRadarQitem(name string) *starr.RadarQueue {
	for _, server := range u.Radarr {
		for _, record := range server.Queue {
			if record.Title == name {
				return record
			}
		}
	}

	return nil
}

// gets a lidarr queue item based on name. returns first match
func (u *Unpackerr) getLidarQitem(name string) *starr.LidarrRecord {
	for _, server := range u.Lidarr {
		for _, record := range server.Queue {
			if record.Title == name {
				return record
			}
		}
	}

	return nil
}
