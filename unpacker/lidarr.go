package unpacker

import (
	"log"
	"time"

	"golift.io/starr"
)

// PollLidarr saves the Lidarr Queue
func (u *Unpackerr) PollLidarr(lidarr *lidarrConfig) error {
	var err error

	lidarr.Lock()
	defer lidarr.Unlock()

	if lidarr.List, err = lidarr.LidarrQueue(1000); err != nil {
		return err
	}

	log.Printf("[Lidarr] Updated (%s): %d Items Queued", lidarr.URL, len(lidarr.List))

	return nil
}

// CheckLidarrQueue passes completed Lidarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) CheckLidarrQueue() {
	check := func(lidarr *lidarrConfig) {
		lidarr.RLock()
		defer lidarr.RUnlock()

		for _, q := range lidarr.List {
			if q.Status == completed && q.Protocol == torrent && !u.historyExists(q.Title) {
				u.HandleCompleted(q.Title, "Lidarr", q.OutputPath)
			} else {
				u.DeLogf("Lidarr: (%s): %s (%s:%d%%): %v",
					lidarr.URL, q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title)
			}
		}
	}

	for _, lidarr := range u.Lidarr {
		check(lidarr)
	}
}

func (u *Unpackerr) handleLidarr(data *Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()

	if item := u.getLidarQitem(name); item != nil {
		u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
		return // We only want finished items.
	}

	if s := u.HandleExtractDone(data, name); s != data.Status {
		data.Status, data.Updated = s, time.Now()
		u.History.Map[name] = data
	}
}

// gets a lidarr queue item based on name. returns first match
func (u *Unpackerr) getLidarQitem(name string) *starr.LidarrRecord {
	getItem := func(name string, lidarr *lidarrConfig) *starr.LidarrRecord {
		lidarr.RLock()
		defer lidarr.RUnlock()

		for i := range lidarr.List {
			if lidarr.List[i].Title == name {
				return lidarr.List[i]
			}
		}

		return nil
	}

	for _, lidarr := range u.Lidarr {
		if s := getItem(name, lidarr); s != nil {
			return s
		}
	}

	return nil
}
