package unpackerr

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golift.io/xtractr"
)

type Metrics struct {
	ExtractTime    *prometheus.HistogramVec
	FilesExtracted *prometheus.CounterVec
	ArchivesRead   *prometheus.CounterVec
	BytesWritten   *prometheus.CounterVec
}

type MetricsCollector struct {
	*Unpackerr
	counter *prometheus.Desc
	gauge   *prometheus.Desc
}

func (c *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.counter
	ch <- c.gauge
}

//nolint:lll
func (c *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.stats()
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Waiting), "waiting")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Queued), "queued")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Extracting), "extracting")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Failed), "failed")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Extracted), "extracted")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Imported), "imported")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(stats.Deleted), "deleted")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(stats.HookOK), "hook_ok")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(stats.HookFail), "hook_fail")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(stats.CmdOK), "cmd_ok")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(stats.CmdFail), "cmd_fail")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(c.Retries), "retries")
	ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(c.Finished), "finished")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(len(c.folders.Events)), "chan_folder_events")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(len(c.updates)), "chan_xtractr_updates")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(len(c.folders.Updates)), "chan_folder_updates")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(len(c.delChan)), "chan_deletes")
	ch <- prometheus.MustNewConstMetric(c.gauge, prometheus.GaugeValue, float64(len(c.hookChan)), "chan_hooks")
}

func (u *Unpackerr) updateMetrics(resp *xtractr.Response, app string) {
	if u.metrics == nil {
		return
	}

	u.metrics.ExtractTime.WithLabelValues(app).Observe(resp.Elapsed.Seconds())
	u.metrics.FilesExtracted.WithLabelValues(app).Add(float64(len(resp.NewFiles)))
	u.metrics.BytesWritten.WithLabelValues(app).Add(float64(resp.Size))
	u.metrics.ArchivesRead.WithLabelValues(app).Add(float64(mapLen(resp.Archives) + mapLen(resp.Extras)))
}

func (u *Unpackerr) setupMetrics() {
	prometheus.MustRegister(&MetricsCollector{
		Unpackerr: u,
		counter:   prometheus.NewDesc("unpackerr_counters", "Unpackerr queue counters", []string{"name"}, nil),
		gauge:     prometheus.NewDesc("unpackerr_gauges", "Unpackerr queue gauges", []string{"name"}, nil),
	})

	u.metrics = &Metrics{
		ExtractTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "unpackerr_extract_time_seconds",
			Help:    "The duration of extractions",
			Buckets: []float64{10, 60, 300, 1800, 3600, 7200, 14400},
		}, []string{"app"}),
		FilesExtracted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_files_extracted_total",
			Help: "The total number files written to disk",
		}, []string{"app"}),
		BytesWritten: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_bytes_written_total",
			Help: "The total number bytes written to disk",
		}, []string{"app"}),
		ArchivesRead: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "unpackerr_archives_read_total",
			Help: "The total number of archive files read",
		}, []string{"app"}),
	}
}

type Stats struct {
	Waiting    uint
	Queued     uint
	Extracting uint
	Failed     uint
	Extracted  uint
	Imported   uint
	Deleted    uint
	HookOK     uint
	HookFail   uint
	CmdOK      uint
	CmdFail    uint
}

func (u *Unpackerr) stats() *Stats {
	stats := &Stats{}
	stats.HookOK, stats.HookFail = u.WebhookCounts()
	stats.CmdOK, stats.CmdFail = u.CmdhookCounts()

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

	return stats
}
