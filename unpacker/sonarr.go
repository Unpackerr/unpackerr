package unpacker

import (
	"log"
	"path/filepath"
	"time"

	"golift.io/starr"
)

// PollSonarr saves the Sonarr Queue
func (u *Unpackerr) PollSonarr(sonarr *sonarrConfig) error {
	var err error

	sonarr.Lock()
	defer sonarr.Unlock()

	if sonarr.List, err = sonarr.SonarrQueue(); err != nil {
		return err
	}

	log.Printf("[Sonarr] Updated (%s): %d Items Queued", sonarr.URL, len(sonarr.List))

	return nil
}

// CheckSonarrQueue passes completed Sonarr-queued downloads to the HandleCompleted method.
func (u *Unpackerr) CheckSonarrQueue() {
	check := func(sonarr *sonarrConfig) {
		sonarr.RLock()
		defer sonarr.RUnlock()

		for _, q := range sonarr.List {
			if q.Status == completed && q.Protocol == torrent && !u.historyExists(q.Title) {
				u.HandleCompleted(q.Title, "Sonarr", filepath.Join(sonarr.Path, q.Title))
			} else {
				u.DeLogf("Sonarr (%s): %s (%s:%d%%): %v (Ep: %v)",
					sonarr.URL, q.Status, q.Protocol, int(100-(q.Sizeleft/q.Size*100)), q.Title, q.Episode.Title)
			}
		}
	}

	for _, sonarr := range u.Sonarr {
		check(sonarr)
	}
}

func (u *Unpackerr) handleSonarr(data *Extracts, name string) {
	u.History.Lock()
	defer u.History.Unlock()

	if item := u.getSonarQitem(name); item != nil {
		u.DeLogf("%s: Item Waiting For Import (%s): %v -> %v", data.App, item.Protocol, name, item.Status)
		return // We only want finished items.
	}

	if s := u.HandleExtractDone(data, name); s != data.Status {
		data.Status, data.Updated = s, time.Now()
		u.History.Map[name] = data
	}
}

func (u *Unpackerr) getSonarQitem(name string) *starr.SonarQueue {
	getItem := func(name string, sonarr *sonarrConfig) *starr.SonarQueue {
		sonarr.RLock()
		defer sonarr.RUnlock()

		for i := range sonarr.List {
			if sonarr.List[i].Title == name {
				return sonarr.List[i]
			}
		}

		return nil
	}

	for _, sonarr := range u.Sonarr {
		if s := getItem(name, sonarr); s != nil {
			return s
		}
	}

	return nil
}
