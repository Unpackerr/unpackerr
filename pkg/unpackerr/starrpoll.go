package unpackerr

import (
	"sync"
	"time"

	"golift.io/starr"
)

// queueView is the subset of a Starr queue record Unpackerr needs to start an extract.
type queueView struct {
	Title, Status, OutputPath string
	Protocol                  starr.Protocol
	Size, Sizeleft            float64
	IDs                       map[string]any
	DebugExtra                string
}

func validateStarrList[T any, P starrApp[T]](unpack *Unpackerr, list *[]P, app starr.App) error {
	tmp := (*list)[:0]

	for idx := range *list {
		if err := unpack.validateApp((*list)[idx].conf(), app); err != nil {
			if skipInvalidApp(err) {
				continue // We ignore these errors, just remove the instance from the list.
			}

			return err
		}

		(*list)[idx].connect()
		tmp = append(tmp, (*list)[idx])
	}

	*list = tmp

	return nil
}

func enqueueStarrPoll[T any, P starrApp[T]](
	unpack *Unpackerr, list []P, app starr.App, start time.Time, wait *sync.WaitGroup,
) {
	for _, server := range list {
		unpack.workChan <- []func(){func() { unpack.getStarrQueue(server, app, start) }, wait.Done}
	}
}

func (u *Unpackerr) getStarrQueue[T any, P starrApp[T]](server P, app starr.App, start time.Time) {
	cfg := server.conf()
	if cfg.APIKey == "" {
		u.Debugf("%s (%s): skipped, no API key", app, cfg.URL)
		return
	}

	total, retrieved, err := server.pollQueue()
	if err != nil {
		u.saveQueueMetrics(0, start, app, cfg.URL, err)
		return
	}

	u.saveQueueMetrics(total, start, app, cfg.URL, nil)

	if !u.Activity || total > 0 {
		u.Printf("[%s] Updated (%s): %d Items Queued, %d Retrieved", app, cfg.URL, total, retrieved)
	}
}

func checkStarrQueue[T any, P starrApp[T]](unpack *Unpackerr, list []P, app starr.App, now time.Time) {
	unpack.lockHistory()
	defer unpack.unlockHistory()

	for _, server := range list {
		cfg := server.conf()

		for _, rec := range server.queueViews() {
			switch item, ok := unpack.Map[rec.Title]; {
			case ok && item.Status == EXTRACTED && unpack.isComplete(rec.Status, rec.Protocol, cfg.Protocols):
				unpack.Debugf("%s (%s): Item Waiting for Import (%s): %v", app, cfg.URL, rec.Protocol, rec.Title)
			case !ok && unpack.isComplete(rec.Status, rec.Protocol, cfg.Protocols) && !unpack.isForgotten(rec.Title):
				waiting := &Extract{
					App:         app,
					URL:         cfg.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  cfg.DeleteOrig,
					DeleteDelay: cfg.DeleteDelay.Duration,
					Syncthing:   cfg.Syncthing,
					MaxBytes:    cfg.maxBytes,
					Path:        unpack.getDownloadPath(rec.OutputPath, app, rec.Title, cfg.Paths),
					IDs:         rec.IDs,
				}
				waiting.XProg = &ExtractProgress{Extract: waiting}
				server.tweakExtract(waiting, rec)
				unpack.Map[rec.Title] = waiting

				fallthrough
			default:
				unpack.Debugf("%s (%s): %s (%s:%d%%): %v%s",
					app, cfg.URL, rec.Status, rec.Protocol,
					percent(rec.Sizeleft, rec.Size), rec.Title, rec.DebugExtra)
			}
		}
	}
}

func haveStarrQitem[T any, P starrApp[T]](list []P, name string) bool {
	for _, server := range list {
		for _, rec := range server.queueViews() {
			if rec.Title == name {
				return true
			}
		}
	}

	return false
}
