package unpacker

import (
	"log"
	"path/filepath"
	"time"

	"golift.io/starr"
)

// PollRadarr saves the Radarr Queue
func (u *Unpackerr) PollRadarr(radarr *radarrConfig) error {
	var err error

	radarr.Lock()
	defer radarr.Unlock()

	if radarr.List, err = radarr.RadarrQueue(); err != nil {
		return err
	}

	log.Printf("[Radarr] Updated (%s): %d Items Queued", radarr.URL, len(radarr.List))

	return nil
}

// CheckRadarrQueue passes completed Radarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) CheckRadarrQueue() {
	check := func(radarr *radarrConfig) {
		radarr.RLock()
		defer radarr.RUnlock()

		for _, q := range radarr.List {
			if q.Status == completed && q.Protocol == torrent {
				go u.HandleCompleted(q.Title, "Radarr", filepath.Join(radarr.Path, q.Title))
			} else {
				u.DeLogf("Radarr (%s): %s (%s:%d%%): %v",
					radarr.URL, q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title)
			}
		}
	}

	for _, radarr := range u.Radarr {
		check(radarr)
	}
}

func (u *Unpackerr) handleRadarr(data *Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()

	if item := u.getRadarQitem(name); item != nil {
		u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
		return // We only want finished items.
	}

	if s := u.HandleExtractDone(data, name); s != data.Status {
		// Status changed.
		data.Status, data.Updated = s, time.Now()
		u.History.Map[name] = data
	}
}

// gets a radarr queue item based on name. returns first match
func (u *Unpackerr) getRadarQitem(name string) *starr.RadarQueue {
	getItem := func(name string, radarr *radarrConfig) *starr.RadarQueue {
		radarr.RLock()
		defer radarr.RUnlock()

		for i := range radarr.List {
			if radarr.List[i].Title == name {
				return radarr.List[i]
			}
		}

		return nil
	}

	for _, radarr := range u.Radarr {
		if s := getItem(name, radarr); s != nil {
			return s
		}
	}

	return nil
}
