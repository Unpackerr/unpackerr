package unpackerr

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golift.io/starr"
)

// queueView is the subset of a Starr queue record Unpackerr needs to start an extract.
type queueView struct {
	Title, Status, OutputPath   string
	TrackedStatus, TrackedState string
	Protocol                    starr.Protocol
	Size, Sizeleft              float64
	IDs                         map[string]any
	DebugExtra                  string
}

func validateStarrList[T any, P starrApp[T]](unpack *Unpackerr, list InstanceMap[T], app starr.App) error {
	for key, item := range list {
		if err := validateInstanceSlug(key); err != nil {
			return err
		}

		if item == nil {
			delete(list, key)
			continue
		}

		server := asStarr[T, P](item)
		if err := unpack.validateApp(server.conf(), app); err != nil {
			if skipInvalidApp(err) {
				delete(list, key)
				continue
			}

			return err
		}

		server.connect()
	}

	return nil
}

func warnDuplicateStarrNames[T any, P starrApp[T]](
	unpack *Unpackerr, seen map[string]string, app starr.App, list InstanceMap[T],
) {
	for _, item := range instanceValues(list) {
		server := asStarr[T, P](item)
		cfg := server.conf()
		name := strings.TrimSpace(cfg.Name)

		if name == "" {
			continue
		}

		key := strings.ToLower(name)
		loc := fmt.Sprintf("%s (%s)", app, cfg.URL)

		if prev, ok := seen[key]; ok {
			unpack.Errorf("Config Warning: duplicate Starr instance name %q on %s and %s; hook exclude cannot tell them apart",
				name, prev, loc)

			continue
		}

		seen[key] = loc
	}
}

func enqueueStarrPoll[T any, P starrApp[T]](
	unpack *Unpackerr, list InstanceMap[T], app starr.App, start time.Time, wait *sync.WaitGroup,
) {
	for _, item := range instanceValues(list) {
		server := asStarr[T, P](item)
		unpack.workChan <- []func(){func() { unpack.getStarrQueue(server, app, start) }, wait.Done}
	}
}

func (u *Unpackerr) getStarrQueue[T any, P starrApp[T]](server P, app starr.App, start time.Time) {
	cfg := server.conf()
	label := cfg.Label(app)

	if cfg.APIKey == "" {
		u.Debugf("%s (%s): skipped, no API key", label, cfg.URL)
		return
	}

	bind, total, retrieved, err := server.pollQueue()
	u.publishStarrPoll(cfg, bind, total, retrieved, err)

	if err != nil {
		u.saveQueueMetrics(0, start, app, cfg.URL, label, err)

		return
	}

	u.saveQueueMetrics(total, start, app, cfg.URL, label, nil)

	if !u.Activity || total > 0 {
		u.Printf("[%s] Updated (%s): %d Items Queued, %d Retrieved", label, cfg.URL, total, retrieved)
	}
}

// publishStarrPoll stores the last poll snapshot under History.mu so HTTP
// stats() and Prometheus Collect cannot race the pointer swap or lastPollErr.
// GetQueue stays outside this lock.
func (u *Unpackerr) publishStarrPoll(cfg *StarrConfig, bind func(), total, retrieved int, err error) {
	u.lockHistory()
	defer u.unlockHistory()

	if err != nil {
		cfg.lastPollErr = err.Error()

		return
	}

	bind()

	cfg.lastQueued = total
	cfg.lastRetrieved = retrieved
	cfg.lastPollErr = ""
}

func checkStarrQueue[T any, P starrApp[T]](unpack *Unpackerr, list InstanceMap[T], app starr.App, now time.Time) {
	unpack.lockHistory()
	defer unpack.unlockHistory()

	for _, item := range instanceValues(list) {
		server := asStarr[T, P](item)
		cfg := server.conf()

		for _, rec := range server.queueViews() {
			item, found := unpack.Map[rec.Title]
			if found && item.App == app && item.URL == cfg.URL {
				item.Name = cfg.Name
			}

			switch {
			case found && item.Status == EXTRACTED && isComplete(rec.Status, rec.Protocol, cfg.Protocols):
				unpack.Debugf("%s (%s): Item Waiting for Import (%s): %v", cfg.Label(app), cfg.URL, rec.Protocol, rec.Title)
			case !found && isComplete(rec.Status, rec.Protocol, cfg.Protocols) && !unpack.isForgotten(rec.Title):
				waiting := &Extract{
					App:         app,
					Name:        cfg.Name,
					URL:         cfg.URL,
					Updated:     now,
					Status:      WAITING,
					DeleteOrig:  cfg.DeleteOrig,
					DeleteDelay: cfg.DeleteDelay.Duration,
					Syncthing:   cfg.Syncthing,
					MaxBytes:    cfg.maxBytes,
					Path:        unpack.getDownloadPath(rec.OutputPath, cfg.Label(app), rec.Title, cfg.Paths),
					IDs:         rec.IDs,
				}
				waiting.XProg = &ExtractProgress{Extract: waiting}
				server.tweakExtract(waiting, rec)
				unpack.Map[rec.Title] = waiting

				fallthrough
			default:
				unpack.Debugf("%s (%s): %s (%s:%d%%): %v%s",
					cfg.Label(app), cfg.URL, rec.Status, rec.Protocol,
					percent(rec.Sizeleft, rec.Size), rec.Title, rec.DebugExtra)
			}
		}
	}
}

func haveStarrQitem[T any, P starrApp[T]](list InstanceMap[T], name string) bool {
	for _, item := range list {
		if item == nil {
			continue
		}

		server := asStarr[T, P](item)
		if server.hasQueueTitle(name) {
			return true
		}
	}

	return false
}

func (u *Unpackerr) starrQueueStats() []StarrQueueStat {
	n := u.starrAppCount()
	if n == 0 {
		return nil
	}

	out := make([]StarrQueueStat, 0, n)
	out = append(out, starrQueueRows[LidarrConfig, *LidarrConfig](u.Lidarr, starr.Lidarr)...)
	out = append(out, starrQueueRows[RadarrConfig, *RadarrConfig](u.Radarr, starr.Radarr)...)
	out = append(out, starrQueueRows[ReadarrConfig, *ReadarrConfig](u.Readarr, starr.Readarr)...)
	out = append(out, starrQueueRows[SonarrConfig, *SonarrConfig](u.Sonarr, starr.Sonarr)...)

	return out
}

func starrQueueRows[T any, P starrApp[T]](list InstanceMap[T], app starr.App) []StarrQueueStat {
	out := make([]StarrQueueStat, 0, len(list))

	for _, item := range instanceValues(list) {
		server := asStarr[T, P](item)
		cfg := server.conf()
		counts := tallyQueueViews(server.queueViews(), cfg.Protocols)
		out = append(out, StarrQueueStat{
			App:         string(app),
			Name:        cfg.Label(app),
			URL:         cfg.URL,
			Queued:      cfg.lastQueued,
			Retrieved:   cfg.lastRetrieved,
			Complete:    counts.complete,
			Match:       counts.match,
			Issues:      counts.issues,
			Downloading: counts.downloading,
			Error:       cfg.lastPollErr,
		})
	}

	return out
}

type queueStatusCounts struct {
	complete, match, issues, downloading int
}

func tallyQueueViews(views []queueView, protocols string) queueStatusCounts {
	var counts queueStatusCounts

	for _, rec := range views {
		status := strings.ToLower(rec.Status)
		tracked := strings.ToLower(rec.TrackedStatus)
		state := strings.ToLower(rec.TrackedState)

		if status == "completed" {
			counts.complete++
		}

		if isComplete(rec.Status, rec.Protocol, protocols) {
			counts.match++
		}

		if status == "failed" || status == "warning" ||
			tracked == "error" || tracked == "warning" ||
			strings.Contains(state, "fail") {
			counts.issues++
		}

		if status == "downloading" {
			counts.downloading++
		}
	}

	return counts
}

func isComplete(status string, protocol starr.Protocol, protos string) bool {
	for s := range strings.FieldsSeq(strings.ReplaceAll(protos, ",", " ")) {
		if strings.EqualFold(string(protocol), s) {
			return strings.EqualFold(status, "completed")
		}
	}

	return false
}
