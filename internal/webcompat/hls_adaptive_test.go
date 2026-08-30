package webcompat

import "testing"

func TestTuneSessionAdjustsOnlyBoundedSpeculativeHeadroom(t *testing.T) {
	manager, err := NewHLSManager(t.TempDir(), DefaultHLSPolicy())
	if err != nil {
		t.Fatalf("new HLS manager: %v", err)
	}
	const sessionID = "adaptive-test"
	if err := manager.PrepareSession(sessionID, 7, 42, "/media/example.mkv", HLSSpec{
		VideoStream: 0, AudioStream: 1, VideoCodec: "h264", AudioCodec: "aac",
		DurationSeconds: 3600, SourceBitrateKbps: 12000,
	}); err != nil {
		t.Fatalf("prepare session: %v", err)
	}

	low, err := manager.TuneSession(7, sessionID, 5, 8)
	if err != nil {
		t.Fatalf("tune low buffer: %v", err)
	}
	if low.AheadBatches != 3 {
		t.Fatalf("expected maximum bounded headroom of 3 batches, got %d", low.AheadBatches)
	}

	healthy, err := manager.TuneSession(7, sessionID, 45, 40)
	if err != nil {
		t.Fatalf("tune healthy buffer: %v", err)
	}
	if healthy.AheadBatches != 1 {
		t.Fatalf("expected normal one-batch headroom, got %d", healthy.AheadBatches)
	}
	if got := adaptiveAheadBatches(sessionID); got != 1 {
		t.Fatalf("stored adaptive headroom mismatch: got %d", got)
	}

	if _, err := manager.TuneSession(8, sessionID, 5, 8); err == nil {
		t.Fatal("expected session owner mismatch to be rejected")
	}
}
