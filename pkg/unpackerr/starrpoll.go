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
