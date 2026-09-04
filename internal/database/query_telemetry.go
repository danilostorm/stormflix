package database

import (
	"sort"
	"sync"
	"time"
)

const querySampleLimit = 512

type queryMetric struct {
	Count    uint64
	Errors   uint64
	Total    time.Duration
	Samples  []time.Duration
	Max      time.Duration
	LastSeen time.Time
}

type QueryMetricSnapshot struct {
	Count      uint64  `json:"count"`
	Errors     uint64  `json:"errors"`
	AverageMS  float64 `json:"average_ms"`
	P95MS      float64 `json:"p95_ms"`
	MaxMS      float64 `json:"max_ms"`
	LastSeenAt string  `json:"last_seen_at,omitempty"`
}

var queryTelemetry = struct {
	sync.Mutex
	metrics map[string]*queryMetric
}{metrics: map[string]*queryMetric{}}

// ObserveQuery records bounded, label-based SQL timing without retaining SQL
// text or parameters. It is intentionally cheap enough for Home hot paths and
// safe to expose in authenticated Admin diagnostics.
func ObserveQuery(label string, duration time.Duration, err error) {
	queryTelemetry.Lock()
	defer queryTelemetry.Unlock()
	metric := queryTelemetry.metrics[label]
	if metric == nil {
		metric = &queryMetric{}
		queryTelemetry.metrics[label] = metric
	}
	metric.Count++
	if err != nil {
		metric.Errors++
	}
	metric.Total += duration
	if duration > metric.Max {
		metric.Max = duration
	}
	metric.LastSeen = time.Now().UTC()
	if len(metric.Samples) < querySampleLimit {
		metric.Samples = append(metric.Samples, duration)
	} else {
		metric.Samples[metric.Count%querySampleLimit] = duration
	}
}

func QueryTelemetrySnapshot() map[string]QueryMetricSnapshot {
	queryTelemetry.Lock()
	defer queryTelemetry.Unlock()
	out := make(map[string]QueryMetricSnapshot, len(queryTelemetry.metrics))
	for label, metric := range queryTelemetry.metrics {
		samples := append([]time.Duration(nil), metric.Samples...)
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		var p95 time.Duration
		if len(samples) > 0 {
			index := (len(samples)*95 + 99) / 100
			if index < 1 {
				index = 1
			}
			p95 = samples[index-1]
		}
		average := time.Duration(0)
		if metric.Count > 0 {
			average = metric.Total / time.Duration(metric.Count)
		}
		out[label] = QueryMetricSnapshot{
			Count: metric.Count, Errors: metric.Errors, AverageMS: durationMS(average),
			P95MS: durationMS(p95), MaxMS: durationMS(metric.Max), LastSeenAt: metric.LastSeen.Format(time.RFC3339),
		}
	}
	return out
}

func durationMS(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000
}
