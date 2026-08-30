package transcode

import (
	"testing"
	"time"
)

func TestSessionIDNamespace(t *testing.T) {
	id := SessionID("abc-123")
	if id != "v5t-abc-123" || !IsSessionID(id) {
		t.Fatalf("unexpected session id %q", id)
	}
	if SessionID(id) != id {
		t.Fatalf("session prefix must be idempotent: %q", SessionID(id))
	}
}

func TestEncoderCandidatesPreferHardwareThenCPU(t *testing.T) {
	m := &Manager{engine: EngineStatus{VideoEncoders: []string{"h264_nvenc", "h264_qsv", "h264_vaapi", "libx264"}, VAAPIDevice: "/dev/dri/renderD128"}}
	items := m.encoderCandidates("h264", false)
	if len(items) != 4 {
		t.Fatalf("expected four candidates, got %#v", items)
	}
	if items[0].name != "h264_nvenc" || items[0].hardware != "nvidia" || items[len(items)-1].name != "libx264" || items[len(items)-1].hardware != "cpu" {
		t.Fatalf("wrong preference order: %#v", items)
	}
}

func TestToneMapUsesSafeCPUEncoderPath(t *testing.T) {
	m := &Manager{engine: EngineStatus{VideoEncoders: []string{"h264_nvenc", "libx264"}, ToneMap: true, ZScale: true}}
	items := m.encoderCandidates("h264", true)
	if len(items) != 1 || items[0].name != "libx264" || items[0].hardware != "cpu" {
		t.Fatalf("tone mapping must avoid incompatible hardware filter surfaces: %#v", items)
	}
}

func TestDefaultPolicyIsSmallBatchAndGloballyBounded(t *testing.T) {
	policy := DefaultPolicy()
	if policy.MaxBytes != 5<<30 {
		t.Fatalf("global cache max = %d", policy.MaxBytes)
	}
	if policy.SegmentDuration != 4*time.Second || policy.BatchSegments != 5 {
		t.Fatalf("unexpected segment policy: %#v", policy)
	}
	if policy.MinFreeBytes < 10<<30 || policy.MinFreePercent < 5 {
		t.Fatalf("free disk reserve was weakened: %#v", policy)
	}
}

func TestSameSpecChangesWhenQualityTargetChanges(t *testing.T) {
	a := Spec{VideoStream: 0, AudioStream: 1, TargetVideoCodec: "h264", TargetAudioCodec: "aac", TargetWidth: 1920, TargetHeight: 1080, TargetBitrateKbps: 8000, DurationSeconds: 100}
	b := a
	if !sameSpec(a, b) {
		t.Fatal("identical specs should reuse session")
	}
	b.TargetHeight = 720
	if sameSpec(a, b) {
		t.Fatal("different target quality must force a new session")
	}
}
