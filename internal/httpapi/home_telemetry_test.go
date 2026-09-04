package httpapi

import (
	"testing"
	"time"
)

func TestHomeTelemetryReportsLatencyAndCacheStates(t *testing.T) {
	var telemetry homeTelemetry
	for i := 1; i <= 100; i++ {
		state := "hit"
		if i%10 == 0 {
			state = "miss"
		}
		telemetry.Observe(time.Duration(i)*time.Millisecond, state)
	}
	snapshot := telemetry.Snapshot()
	if snapshot.RequestCount != 100 || snapshot.WindowSize != 100 {
		t.Fatalf("unexpected counters: %+v", snapshot)
	}
	if snapshot.CacheStates["hit"] != 90 || snapshot.CacheStates["miss"] != 10 {
		t.Fatalf("unexpected cache states: %+v", snapshot.CacheStates)
	}
	if snapshot.P50MS < 49 || snapshot.P50MS > 51 || snapshot.P95MS < 94 || snapshot.P95MS > 96 || snapshot.LastMS != 100 {
		t.Fatalf("unexpected latency distribution: %+v", snapshot)
	}
}

func TestHomeTelemetryKeepsBoundedWindow(t *testing.T) {
	var telemetry homeTelemetry
	for i := 0; i < homeTelemetryWindow+20; i++ {
		telemetry.Observe(time.Millisecond, "hit")
	}
	snapshot := telemetry.Snapshot()
	if snapshot.RequestCount != homeTelemetryWindow+20 || snapshot.WindowSize != homeTelemetryWindow {
		t.Fatalf("telemetry window is not bounded: %+v", snapshot)
	}
}

func TestHomeTelemetryReportsClientFirstContent(t *testing.T) {
	var telemetry homeTelemetry
	for i := 1; i <= 100; i++ {
		telemetry.ObserveClient(time.Duration(i)*time.Millisecond, time.Duration(i/2)*time.Millisecond, 10*time.Millisecond)
	}
	snapshot := telemetry.Snapshot()
	if snapshot.Client.Count != 100 || snapshot.Client.WindowSize != 100 || snapshot.Client.FirstContentP95MS < 94 || snapshot.Client.FirstContentP95MS > 96 {
		t.Fatalf("unexpected client telemetry: %+v", snapshot.Client)
	}
}
