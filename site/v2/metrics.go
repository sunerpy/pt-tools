// Package v2 provides metrics collection for performance monitoring
package v2

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Counter is a monotonically increasing counter
type Counter struct {
	value int64
}

// NewCounter creates a new counter
func NewCounter() *Counter {
	return &Counter{}
}

// Inc increments the counter by 1
func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

// Add adds the given value to the counter
func (c *Counter) Add(delta int64) {
	atomic.AddInt64(&c.value, delta)
}

// Value returns the current counter value
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

// Gauge is a value that can go up and down
type Gauge struct {
	value int64
}

// NewGauge creates a new gauge
func NewGauge() *Gauge {
	return &Gauge{}
}

// Set sets the gauge value
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Inc increments the gauge by 1
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge by 1
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Add adds the given value to the gauge
func (g *Gauge) Add(delta int64) {
	atomic.AddInt64(&g.value, delta)
}

// Value returns the current gauge value
func (g *Gauge) Value() int64 {
	return atomic.LoadInt64(&g.value)
}

// Histogram tracks the distribution of values
type Histogram struct {
	mu     sync.RWMutex
	count  int64
	sum    float64
	min    float64
	max    float64
	values []float64 // For percentile calculation (limited size)
}

// NewHistogram creates a new histogram
func NewHistogram() *Histogram {
	return &Histogram{
		min:    float64(^uint64(0) >> 1), // Max float64
		values: make([]float64, 0, 1000),
	}
}

// Observe records a value
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += value

	if value < h.min {
		h.min = value
	}
	if value > h.max {
		h.max = value
	}

	// Keep last 1000 values for percentile calculation
	if len(h.values) < 1000 {
		h.values = append(h.values, value)
	} else {
		// Rotate: remove oldest, add newest
		copy(h.values, h.values[1:])
		h.values[len(h.values)-1] = value
	}
}

// Stats returns histogram statistics
func (h *Histogram) Stats() HistogramStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := HistogramStats{
		Count: h.count,
		Sum:   h.sum,
	}

	if h.count > 0 {
		stats.Min = h.min
		stats.Max = h.max
		stats.Avg = h.sum / float64(h.count)
	}

	return stats
}

// HistogramStats contains histogram statistics
type HistogramStats struct {
	Count int64   `json:"count"`
	Sum   float64 `json:"sum"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Avg   float64 `json:"avg"`
}

// Timer measures elapsed time
type Timer struct {
	histogram *Histogram
	startTime time.Time
}

// NewTimer creates a new timer
func NewTimer(h *Histogram) *Timer {
	return &Timer{
		histogram: h,
		startTime: time.Now(),
	}
}

// ObserveDuration records the elapsed time since the timer was created
func (t *Timer) ObserveDuration() time.Duration {
	duration := time.Since(t.startTime)
	t.histogram.Observe(float64(duration.Milliseconds()))
	return duration
}

// MetricsRegistry manages all metrics
type MetricsRegistry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// Counter returns or creates a counter with the given name
func (r *MetricsRegistry) Counter(name string) *Counter {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()

	if ok {
		return c
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existingC, exists := r.counters[name]; exists {
		return existingC
	}

	c = NewCounter()
	r.counters[name] = c
	return c
}

// Gauge returns or creates a gauge with the given name
func (r *MetricsRegistry) Gauge(name string) *Gauge {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()

	if ok {
		return g
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existingG, exists := r.gauges[name]; exists {
		return existingG
	}

	g = NewGauge()
	r.gauges[name] = g
	return g
}

// Histogram returns or creates a histogram with the given name
func (r *MetricsRegistry) Histogram(name string) *Histogram {
	r.mu.RLock()
	h, ok := r.histograms[name]
	r.mu.RUnlock()

	if ok {
		return h
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existingH, exists := r.histograms[name]; exists {
		return existingH
	}

	h = NewHistogram()
	r.histograms[name] = h
	return h
}

// Timer creates a new timer for the given histogram
func (r *MetricsRegistry) Timer(name string) *Timer {
	return NewTimer(r.Histogram(name))
}

// Snapshot returns a snapshot of all metrics
func (r *MetricsRegistry) Snapshot() MetricsSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := MetricsSnapshot{
		Counters:   make(map[string]int64),
		Gauges:     make(map[string]int64),
		Histograms: make(map[string]HistogramStats),
		Timestamp:  time.Now(),
	}

	for name, c := range r.counters {
		snapshot.Counters[name] = c.Value()
	}

	for name, g := range r.gauges {
		snapshot.Gauges[name] = g.Value()
	}

	for name, h := range r.histograms {
		snapshot.Histograms[name] = h.Stats()
	}

	return snapshot
}

// MetricsSnapshot contains a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	Counters   map[string]int64          `json:"counters"`
	Gauges     map[string]int64          `json:"gauges"`
	Histograms map[string]HistogramStats `json:"histograms"`
	Timestamp  time.Time                 `json:"timestamp"`
}

// SiteMetrics provides site-specific metrics
type SiteMetrics struct {
	registry *MetricsRegistry
}

// NewSiteMetrics creates a new site metrics instance
func NewSiteMetrics(registry *MetricsRegistry) *SiteMetrics {
	if registry == nil {
		registry = NewMetricsRegistry()
	}
	return &SiteMetrics{registry: registry}
}

// RecordRequest records a site request
func (m *SiteMetrics) RecordRequest(site string, success bool, duration time.Duration) {
	m.registry.Counter("site_requests_total").Inc()
	m.registry.Counter("site_requests_" + site + "_total").Inc()

	if success {
		m.registry.Counter("site_requests_success_total").Inc()
		m.registry.Counter("site_requests_" + site + "_success").Inc()
	} else {
		m.registry.Counter("site_requests_failure_total").Inc()
		m.registry.Counter("site_requests_" + site + "_failure").Inc()
	}

	m.registry.Histogram("site_request_duration_ms").Observe(float64(duration.Milliseconds()))
	m.registry.Histogram("site_request_duration_" + site + "_ms").Observe(float64(duration.Milliseconds()))
}

// RecordCacheHit records a cache hit
func (m *SiteMetrics) RecordCacheHit(cacheType string) {
	m.registry.Counter("cache_hits_total").Inc()
	m.registry.Counter("cache_hits_" + cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *SiteMetrics) RecordCacheMiss(cacheType string) {
	m.registry.Counter("cache_misses_total").Inc()
	m.registry.Counter("cache_misses_" + cacheType).Inc()
}

// RecordDownloaderRequest records a downloader request
func (m *SiteMetrics) RecordDownloaderRequest(downloader string, success bool, duration time.Duration) {
	m.registry.Counter("downloader_requests_total").Inc()
	m.registry.Counter("downloader_requests_" + downloader + "_total").Inc()

	if success {
		m.registry.Counter("downloader_requests_success_total").Inc()
	} else {
		m.registry.Counter("downloader_requests_failure_total").Inc()
	}

	m.registry.Histogram("downloader_request_duration_ms").Observe(float64(duration.Milliseconds()))
}

// RecordError records an error by type
func (m *SiteMetrics) RecordError(errorType string) {
	m.registry.Counter("errors_total").Inc()
	m.registry.Counter("errors_" + errorType).Inc()
}

// SetActiveSites sets the number of active sites
func (m *SiteMetrics) SetActiveSites(count int) {
	m.registry.Gauge("active_sites").Set(int64(count))
}

// SetActiveDownloaders sets the number of active downloaders
func (m *SiteMetrics) SetActiveDownloaders(count int) {
	m.registry.Gauge("active_downloaders").Set(int64(count))
}

// Snapshot returns a snapshot of all metrics
func (m *SiteMetrics) Snapshot() MetricsSnapshot {
	return m.registry.Snapshot()
}

// DefaultMetrics is the default metrics registry
var DefaultMetrics = NewMetricsRegistry()

// DefaultSiteMetrics is the default site metrics instance
var DefaultSiteMetrics = NewSiteMetrics(DefaultMetrics)
