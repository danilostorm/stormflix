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
	client    clientHomeTelemetry
}

type clientHomeTelemetry struct {
	firstContent [homeTelemetryWindow]time.Duration
	response     [homeTelemetryWindow]time.Duration
	render       [homeTelemetryWindow]time.Duration
	next         int
	count        uint64
}

type homeTelemetrySnapshot struct {
	RequestCount uint64             `json:"request_count"`
	WindowSize   int                `json:"window_size"`
	P50MS        float64            `json:"p50_ms"`
	P95MS        float64            `json:"p95_ms"`
	P99MS        float64            `json:"p99_ms"`
	AverageMS    float64            `json:"average_ms"`
	LastMS       float64            `json:"last_ms"`
	LastAt       string             `json:"last_at,omitempty"`
	CacheStates  map[string]uint64  `json:"cache_states"`
	Client       homeClientSnapshot `json:"client"`
}

type homeClientSnapshot struct {
	Count             uint64  `json:"count"`
	WindowSize        int     `json:"window_size"`
	FirstContentP95MS float64 `json:"first_content_p95_ms"`
	ResponseP95MS     float64 `json:"response_p95_ms"`
	RenderP95MS       float64 `json:"render_p95_ms"`
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

func (h *homeTelemetry) ObserveClient(firstContent, response, render time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.client.firstContent[h.client.next] = firstContent
	h.client.response[h.client.next] = response
	h.client.render[h.client.next] = render
	h.client.next = (h.client.next + 1) % homeTelemetryWindow
	h.client.count++
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
	clientSize := int(h.client.count)
	if clientSize > homeTelemetryWindow {
		clientSize = homeTelemetryWindow
	}
	out.Client.Count = h.client.count
	out.Client.WindowSize = clientSize
	if clientSize > 0 {
		first := append([]time.Duration(nil), h.client.firstContent[:clientSize]...)
		response := append([]time.Duration(nil), h.client.response[:clientSize]...)
		render := append([]time.Duration(nil), h.client.render[:clientSize]...)
		sort.Slice(first, func(i, j int) bool { return first[i] < first[j] })
		sort.Slice(response, func(i, j int) bool { return response[i] < response[j] })
		sort.Slice(render, func(i, j int) bool { return render[i] < render[j] })
		out.Client.FirstContentP95MS = durationMS(percentileDuration(first, .95))
		out.Client.ResponseP95MS = durationMS(percentileDuration(response, .95))
		out.Client.RenderP95MS = durationMS(percentileDuration(render, .95))
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
