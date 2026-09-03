package transcode

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestProcessSchedulerBoundsTotalAndVideoWork(t *testing.T) {
	scheduler := NewProcessScheduler(2, 1)
	releaseVideo, _, err := scheduler.Acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	releaseRemux, _, err := scheduler.Acquire(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, _, err := scheduler.Acquire(ctx, false); err == nil {
		t.Fatal("third process should wait for the total limit")
	}
	if status := scheduler.Status(); status.Active != 2 || status.VideoActive != 1 {
		t.Fatalf("unexpected active resources: %#v", status)
	}

	releaseVideo()
	releaseVideo() // release callbacks are deliberately idempotent
	releaseRemux()
	if status := scheduler.Status(); status.Active != 0 || status.VideoActive != 0 {
		t.Fatalf("resources leaked after release: %#v", status)
	}
}

func TestProcessSchedulerVideoWaitDoesNotLeakSlot(t *testing.T) {
	scheduler := NewProcessScheduler(2, 1)
	release, _, err := scheduler.Acquire(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := scheduler.Acquire(ctx, true); err == nil {
		t.Fatal("second video process should be bounded")
	}
	release()
	if status := scheduler.Status(); status.Active != 0 || status.VideoActive != 0 {
		t.Fatalf("video resource leaked: %#v", status)
	}
}

func TestCPUThreadLimitIsReportedAndUsedBySoftwareEncoder(t *testing.T) {
	ConfigureCPUThreadLimit(3)
	defer ConfigureCPUThreadLimit(6)
	want := 3
	if runtime.NumCPU() < want {
		want = runtime.NumCPU()
	}
	if status := ProcessResources(); status.CPUThreadLimit != want {
		t.Fatalf("CPU thread limit=%d, want %d", status.CPUThreadLimit, want)
	}
	args := encoderArgs(encoderCandidate{name: "libx264", hardware: "cpu"}, Spec{TargetBitrateKbps: 8000})
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-threads" && args[i+1] == fmt.Sprint(want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("software encoder args do not enforce thread budget: %v", args)
	}
}
