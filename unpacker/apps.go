package unpacker

import (
	"path/filepath"

	"golift.io/starr"
)

// checkSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) checkSonarrQueue() {
	for _, server := range u.Sonarr {
		for _, q := range server.Queue {
			if _, ok := u.Map[q.Title]; !ok && q.Status == completed && q.Protocol == torrent {
				u.handleCompletedDownload(q.Title, "Sonarr", filepath.Join(server.Path, q.Title))
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
