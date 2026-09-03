package httpapi

import (
	"sort"
	"sync"
	"time"
)

const homeTelemetryWindow = 256

type homeTelemetry struct {
	mu        sync.Mutex
	durations [homeTelemetryWindow]time.Duration
	next      int
	count     uint64
	states    map[string]uint64
	last      time.Duration
	lastAt    time.Time
}

type homeTelemetrySnapshot struct {
	RequestCount uint64            `json:"request_count"`
	WindowSize   int               `json:"window_size"`
	P50MS        float64           `json:"p50_ms"`
	P95MS        float64           `json:"p95_ms"`
	P99MS        float64           `json:"p99_ms"`
	AverageMS    float64           `json:"average_ms"`
	LastMS       float64           `json:"last_ms"`
	LastAt       string            `json:"last_at,omitempty"`
	CacheStates  map[string]uint64 `json:"cache_states"`
}

func (h *homeTelemetry) Observe(duration time.Duration, state string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.states == nil {
		h.states = map[string]uint64{}
	}
	if state == "" {
		state = "unknown"
	}
	h.states[state]++
	h.durations[h.next] = duration
	h.next = (h.next + 1) % len(h.durations)
	h.count++
	h.last = duration
	h.lastAt = time.Now().UTC()
}

func (h *homeTelemetry) Snapshot() homeTelemetrySnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()

	sampleSize := int(h.count)
	if sampleSize > len(h.durations) {
		sampleSize = len(h.durations)
	}
	samples := make([]time.Duration, sampleSize)
	copy(samples, h.durations[:sampleSize])
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	states := make(map[string]uint64, len(h.states))
	for state, count := range h.states {
		states[state] = count
	}
	out := homeTelemetrySnapshot{
		RequestCount: h.count, WindowSize: sampleSize, LastMS: durationMS(h.last), CacheStates: states,
	}
	if !h.lastAt.IsZero() {
		out.LastAt = h.lastAt.Format(time.RFC3339)
	}
	if sampleSize == 0 {
		return out
	}
	var total time.Duration
	for _, sample := range samples {
		total += sample
	}
	out.AverageMS = durationMS(total / time.Duration(sampleSize))
	out.P50MS = durationMS(percentileDuration(samples, 0.50))
	out.P95MS = durationMS(percentileDuration(samples, 0.95))
	out.P99MS = durationMS(percentileDuration(samples, 0.99))
	return out
}

func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * percentile)
	return sorted[index]
}

func durationMS(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}
