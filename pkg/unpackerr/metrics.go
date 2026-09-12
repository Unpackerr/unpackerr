package unpackerr

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golift.io/starr"
	"golift.io/version"
	"golift.io/xtractr"
)

// metrics holds the non-custom Prometheus collector metrics for the app.
type metrics struct {
	AppQueueErr    *prometheus.CounterVec
	AppQueueGet    *prometheus.CounterVec
	AppQueues      *prometheus.GaugeVec
	AppRequests    *prometheus.GaugeVec
	ArchivesRead   *prometheus.CounterVec
	BytesWritten   *prometheus.CounterVec
	ExtractTime    *prometheus.HistogramVec
	FilesExtracted *prometheus.CounterVec
	Uptime         prometheus.CounterFunc
}

// MetricsCollector is used to plug into a custom Prometheus metrics collector.
type MetricsCollector struct {
	*Unpackerr
	counter *prometheus.Desc
	gauge   *prometheus.Desc
	buffer  *prometheus.Desc
}

// Describe satisfies the Prometheus custom metrics collector.
func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.counter, c.gauge, c.buffer} {
		ch <- desc
	}
}

// Collect satisfies the Prometheus custom metrics collector.
//
//nolint:wsl_v5
func (c *MetricsCollector) Collect(metrics chan<- prometheus.Metric) {
	stats := c.stats()
	newMetric := prometheus.MustNewConstMetric

	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Waiting), "waiting")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Queued), "queued")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Extracting), "extracting")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Failed), "failed")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Extracted), "extracted")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Imported), "imported")
	metrics <- newMetric(c.gauge, prometheus.GaugeValue, float64(stats.Deleted), "deleted")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.HookOK), "hook_ok")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.HookFail), "hook_fail")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.CmdOK), "cmd_ok")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.CmdFail), "cmd_fail")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.Retries), "retries")
	metrics <- newMetric(c.counter, prometheus.CounterValue, float64(stats.Finished), "finished")
	metrics <- newMetric(c.buffer, prometheus.GaugeValue, float64(len(c.folders.Events)), "folder_events")
	metrics <- newMetric(c.buffer, prometheus.GaugeValue, float64(len(c.updates)), "xtractr_updates")
	metrics <- newMetric(c.buffer, prometheus.GaugeValue, float64(len(c.folders.Updates)), "folder_updates")
	metrics <- newMetric(c.buffer, prometheus.GaugeValue, float64(len(c.delChan)), "deletes")
	metrics <- newMetric(c.buffer, prometheus.GaugeValue, float64(c.hookWorker.Len()), "hooks")
}

// updateMetrics observes metrics for each completed extraction. The url for a folder is the watch path.
func (u *Unpackerr) updateMetrics(resp *xtractr.Response, app starr.App, url string) {
	if u.metrics == nil {
		return
	}

	u.metrics.ArchivesRead.WithLabelValues(string(app), url).Add(float64(resp.Archives.Count() + resp.Extras.Count()))
	u.metrics.BytesWritten.WithLabelValues(string(app), url).Add(float64(resp.Size))
	u.metrics.ExtractTime.WithLabelValues(string(app), url).Observe(resp.Elapsed.Seconds())
	u.metrics.FilesExtracted.WithLabelValues(string(app), url).Add(float64(len(resp.NewFiles)))
}

// saveQueueMetrics observes metrics for each starr app queue request.
// app is the dialect for Prometheus labels; label is the human instance name for logs.
func (u *Unpackerr) saveQueueMetrics(size int, start time.Time, app starr.App, url, label string, err error) {
	if err != nil {
		if label == "" {
			label = string(app)
		}

		u.Errorf("%s (%s): %v", label, url, err)
	}

	if u.metrics == nil {
		return
	}

	if err != nil {
		u.metrics.AppQueueErr.WithLabelValues(string(app), url).Inc()
	}

	u.metrics.AppQueueGet.WithLabelValues(string(app), url).Inc()
	u.metrics.AppQueues.WithLabelValues(string(app), url).Set(float64(size))
	u.metrics.AppRequests.WithLabelValues(string(app), url).Set(time.Since(start).Seconds())
}

// setupMetrics is called once on startup if metrics are enabled.
func (u *Unpackerr) setupMetrics() {
	prometheus.MustRegister(&MetricsCollector{
		Unpackerr: u,
		counter:   prometheus.NewDesc("unpackerr_counters", "Unpackerr queue counters", []string{"name"}, nil),
		gauge:     prometheus.NewDesc("unpackerr_gauges", "Unpackerr queue gauges", []string{"name"}, nil),
		buffer:    prometheus.NewDesc("unpackerr_buffers", "Unpackerr channel buffer gauges", []string{"name"}, nil),
	})

	u.metrics = &metrics{
		AppQueueErr: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_app_queue_fetch_errors_total",
			Help: "Total times the starr activity queue fetch returned an error",
		}, []string{"app", "url"}),
		AppQueueGet: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_app_queue_fetch_total",
			Help: "Total times the starr activity queue was fetched",
		}, []string{"app", "url"}),
		AppQueues: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "unpackerr_app_queue_size",
			Help: "The total number of items queued in a Starr app",
		}, []string{"app", "url"}),
		AppRequests: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "unpackerr_app_queue_fetch_time_seconds",
			Help: "The duration of queue fetch API requests to Starr apps",
		}, []string{"app", "url"}),
		ArchivesRead: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_archives_read_total",
			Help: "The total number of archive files read",
		}, []string{"app", "url"}),
		BytesWritten: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_bytes_written_total",
			Help: "The total number bytes written to disk",
		}, []string{"app", "url"}),
		ExtractTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "unpackerr_extract_time_seconds",
			Help:    "The duration of extractions",
			Buckets: []float64{10, 60, 300, 1800, 3600, 7200, 14400},
		}, []string{"app", "url"}),
		FilesExtracted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_files_extracted_total",
			Help: "The total number files written to disk",
		}, []string{"app", "url"}),
		Uptime: promauto.NewCounterFunc(prometheus.CounterOpts{
			Name: "unpackerr_uptime_seconds_total",
			Help: "Duration Unpackerr has been running in seconds",
		}, func() float64 { return time.Since(version.Started).Seconds() }),
	}
}

// Stats is filled and returned when a stats request is issued.
type Stats struct {
	Waiting      uint             `json:"waiting"`
	Queued       uint             `json:"queued"`
	Extracting   uint             `json:"extracting"`
	Failed       uint             `json:"failed"`
	Extracted    uint             `json:"extracted"`
	Imported     uint             `json:"imported"`
	Deleted      uint             `json:"deleted"`
	HookOK       uint             `json:"hookOK"`
	HookFail     uint             `json:"hookFail"`
	CmdOK        uint             `json:"cmdOK"`
	CmdFail      uint             `json:"cmdFail"`
	Retries      uint             `json:"retries"`
	Finished     uint             `json:"finished"`
	Starrs       uint             `json:"starrs"`
	Folders      uint             `json:"folders"`
	Webhooks     uint             `json:"webhooks"`
	Cmdhooks     uint             `json:"cmdhooks"`
	StackFS      BufferStat       `json:"stackFS"`
	StackXtractr BufferStat       `json:"stackXtractr"`
	StackFolder  BufferStat       `json:"stackFolder"`
	StackHook    BufferStat       `json:"stackHook"`
	StackDel     BufferStat       `json:"stackDel"`
	StackTask    BufferStat       `json:"stackTask"`
	StarrQueues  []StarrQueueStat `json:"starrQueues,omitempty"`
}

// BufferStat is occupancy and capacity of one internal channel.
type BufferStat struct {
	Len uint `json:"len"`
	Cap uint `json:"cap"`
}

func (s *Stats) stackTotal() uint {
	if s == nil {
		return 0
	}

	return s.StackFS.Len + s.StackXtractr.Len + s.StackFolder.Len +
		s.StackHook.Len + s.StackDel.Len + s.StackTask.Len
}

func chanStat[T any](ch <-chan T) BufferStat {
	return BufferStat{Len: uint(len(ch)), Cap: uint(cap(ch))}
}

// StarrQueueStat is one Starr instance's last activity-queue poll.
type StarrQueueStat struct {
	App         string `json:"app"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	Queued      int    `json:"queued"`
	Retrieved   int    `json:"retrieved"`
	Complete    int    `json:"complete"`
	Match       int    `json:"match"`
	Issues      int    `json:"issues"`
	Downloading int    `json:"downloading"`
	Error       string `json:"error,omitempty"`
}

// stats compiles and builds the statistics for the app.
func (u *Unpackerr) stats() *Stats {
	stats := &Stats{}

	u.rLockHistory()
	defer u.rUnlockHistory()

	u.fillQueueStats(stats)

	return stats
}

func (u *Unpackerr) fillQueueStats(stats *Stats) {
	stats.Retries = u.Retries
	stats.Finished = u.Finished
	u.configMu.RLock()
	stats.Starrs = uint(u.starrAppCount())
	stats.Folders = uint(len(u.Folders))
	stats.StarrQueues = u.starrQueueStats()
	u.configMu.RUnlock()
	stats.Webhooks = uint(len(u.hookList()))
	stats.Cmdhooks = uint(len(u.cmdhookList()))
	stats.HookOK, stats.HookFail = u.WebhookCounts()
	stats.CmdOK, stats.CmdFail = u.CmdhookCounts()
	u.fillStackDepths(stats)

	for name := range u.Map {
		switch u.Map[name].Status {
		case WAITING:
			stats.Waiting++
		case QUEUED:
			stats.Queued++
		case EXTRACTING:
			stats.Extracting++
		case DELETEFAILED, EXTRACTFAILED:
			stats.Failed++
		case EXTRACTED:
			stats.Extracted++
		case DELETED, DELETING:
			stats.Deleted++
		case IMPORTED:
			stats.Imported++
		}
	}
}

// fillStackDepths is channel occupancy for the totals log and /api/stats stacks.
func (u *Unpackerr) fillStackDepths(stats *Stats) {
	stats.StackXtractr = chanStat(u.updates)
	stats.StackDel = chanStat(u.delChan)
	stats.StackTask = chanStat(u.taskChan)

	if u.folders != nil {
		stats.StackFS = chanStat(u.folders.Events)
		stats.StackFolder = chanStat(u.folders.Updates)
	}

	if u.hookWorker != nil {
		stats.StackHook = BufferStat{Len: uint(u.hookWorker.Len()), Cap: uint(u.hookWorker.Cap())}
	}
}
