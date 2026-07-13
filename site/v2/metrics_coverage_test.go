package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCounter_IncAddValue(t *testing.T) {
	c := NewCounter()
	assert.Equal(t, int64(0), c.Value())
	c.Inc()
	c.Inc()
	assert.Equal(t, int64(2), c.Value())
	c.Add(5)
	assert.Equal(t, int64(7), c.Value())
}

func TestGauge_SetIncDecAdd(t *testing.T) {
	g := NewGauge()
	g.Set(10)
	assert.Equal(t, int64(10), g.Value())
	g.Inc()
	assert.Equal(t, int64(11), g.Value())
	g.Dec()
	assert.Equal(t, int64(10), g.Value())
	g.Add(5)
	assert.Equal(t, int64(15), g.Value())
	g.Add(-20)
	assert.Equal(t, int64(-5), g.Value())
}

func TestHistogram_ObserveStats(t *testing.T) {
	h := NewHistogram()
	// empty stats
	empty := h.Stats()
	assert.Equal(t, int64(0), empty.Count)

	h.Observe(10)
	h.Observe(20)
	h.Observe(30)
	stats := h.Stats()
	assert.Equal(t, int64(3), stats.Count)
	assert.InDelta(t, 60.0, stats.Sum, 0.001)
	assert.InDelta(t, 10.0, stats.Min, 0.001)
	assert.InDelta(t, 30.0, stats.Max, 0.001)
	assert.InDelta(t, 20.0, stats.Avg, 0.001)
}

func TestHistogram_Rotation(t *testing.T) {
	h := NewHistogram()
	// Push more than 1000 to trigger rotation branch.
	for i := 0; i < 1050; i++ {
		h.Observe(float64(i))
	}
	stats := h.Stats()
	assert.Equal(t, int64(1050), stats.Count)
	assert.InDelta(t, 0.0, stats.Min, 0.001)
	assert.InDelta(t, 1049.0, stats.Max, 0.001)
	assert.Len(t, h.values, 1000)
}

func TestTimer_ObserveDuration(t *testing.T) {
	h := NewHistogram()
	timer := NewTimer(h)
	time.Sleep(2 * time.Millisecond)
	dur := timer.ObserveDuration()
	assert.Greater(t, dur, time.Duration(0))
	assert.Equal(t, int64(1), h.Stats().Count)
}

func TestMetricsRegistry_ReuseInstances(t *testing.T) {
	r := NewMetricsRegistry()

	c1 := r.Counter("req")
	c2 := r.Counter("req")
	assert.Same(t, c1, c2)

	g1 := r.Gauge("active")
	g2 := r.Gauge("active")
	assert.Same(t, g1, g2)

	h1 := r.Histogram("lat")
	h2 := r.Histogram("lat")
	assert.Same(t, h1, h2)

	timer := r.Timer("lat")
	require.NotNil(t, timer)
	timer.ObserveDuration()
}

func TestMetricsRegistry_Snapshot(t *testing.T) {
	r := NewMetricsRegistry()
	r.Counter("c").Add(3)
	r.Gauge("g").Set(7)
	r.Histogram("h").Observe(5)

	snap := r.Snapshot()
	assert.Equal(t, int64(3), snap.Counters["c"])
	assert.Equal(t, int64(7), snap.Gauges["g"])
	assert.Equal(t, int64(1), snap.Histograms["h"].Count)
	assert.False(t, snap.Timestamp.IsZero())
}

func TestSiteMetrics_RecordAll(t *testing.T) {
	m := NewSiteMetrics(nil) // nil -> creates its own registry

	m.RecordRequest("hdsky", true, 100*time.Millisecond)
	m.RecordRequest("hdsky", false, 50*time.Millisecond)
	m.RecordCacheHit("search")
	m.RecordCacheMiss("search")
	m.RecordDownloaderRequest("qb", true, 10*time.Millisecond)
	m.RecordDownloaderRequest("qb", false, 10*time.Millisecond)
	m.RecordError("timeout")
	m.SetActiveSites(4)
	m.SetActiveDownloaders(2)

	snap := m.Snapshot()
	assert.Equal(t, int64(2), snap.Counters["site_requests_total"])
	assert.Equal(t, int64(1), snap.Counters["site_requests_success_total"])
	assert.Equal(t, int64(1), snap.Counters["site_requests_failure_total"])
	assert.Equal(t, int64(1), snap.Counters["cache_hits_total"])
	assert.Equal(t, int64(1), snap.Counters["cache_misses_total"])
	assert.Equal(t, int64(2), snap.Counters["downloader_requests_total"])
	assert.Equal(t, int64(1), snap.Counters["errors_total"])
	assert.Equal(t, int64(4), snap.Gauges["active_sites"])
	assert.Equal(t, int64(2), snap.Gauges["active_downloaders"])
}

func TestMetricType_Constants(t *testing.T) {
	assert.Equal(t, MetricType("counter"), MetricTypeCounter)
	assert.Equal(t, MetricType("gauge"), MetricTypeGauge)
	assert.Equal(t, MetricType("histogram"), MetricTypeHistogram)
}
