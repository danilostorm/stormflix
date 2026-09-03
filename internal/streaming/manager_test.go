package streaming

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danilostorm/stormflix/internal/transcode"
)

func TestCPUH264FallbackUsesLiveSuperfastPreset(t *testing.T) {
	transcode.ConfigureCPUThreadLimit(3)
	defer transcode.ConfigureCPUThreadLimit(6)
	args := encoderArgs(encoderCandidate{name: "libx264", hardware: "cpu"}, Spec{TargetBitrateKbps: 8000})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-preset superfast") || !strings.Contains(joined, "-threads 3") {
		t.Fatalf("expected low-CPU superfast live preset, got %q", joined)
	}
}

func TestHDRContinuousStreamUsesNVENCWithCPUFallback(t *testing.T) {
	m := &Manager{engine: transcode.EngineStatus{VideoEncoders: []string{"h264_nvenc", "h264_qsv", "libx264"}}}
	items := m.encoderCandidates(Spec{TargetVideoCodec: "h264", VideoTranscode: true, ToneMap: true})
	if len(items) != 2 || items[0].name != "h264_nvenc" || items[1].name != "libx264" {
		t.Fatalf("unexpected HDR encoder order: %#v", items)
	}
}

func TestDefaultPolicyBoundsContinuousWebStreamCache(t *testing.T) {
	policy := DefaultPolicy()
	if policy.MaxBytes != 5<<30 || policy.MinFreeBytes < 10<<30 || policy.MinFreePercent < 5 {
		t.Fatalf("disk safety defaults were weakened: %#v", policy)
	}
	if got := time.Duration(policy.MaxAheadSegments) * segmentDuration; got > time.Minute {
		t.Fatalf("continuous worker may run too far ahead: %s", got)
	}
	if got := time.Duration(policy.KeepBehindSegments) * segmentDuration; got > 4*time.Minute {
		t.Fatalf("continuous worker retains too much history: %s", got)
	}
}

func TestSetPlaybackStateUpdatesAutomaticWorkerControl(t *testing.T) {
	m := &Manager{dir: filepath.Join(t.TempDir(), "cache"), policy: DefaultPolicy(), sessions: map[string]*session{}}
	id := SessionID("state-test")
	now := time.Now()
	m.sessions[id] = &session{ID: id, UserID: 7, MediaID: 8, LastTouch: now}
	m.SetPlaybackState(7, id, "PAUSED", 42)
	s := m.sessions[id]
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PlaybackState != "paused" {
		t.Fatalf("state=%q, want paused", s.PlaybackState)
	}
	if s.RequestedSegment != 21 {
		t.Fatalf("requested segment=%d, want 21", s.RequestedSegment)
	}
}

func TestNormalizePolicyKeepsAheadAndResumeThresholdsSafe(t *testing.T) {
	policy := normalizePolicy(Policy{MaxAheadSegments: 3, ResumeAheadSegments: 9})
	if policy.ResumeAheadSegments >= policy.MaxAheadSegments {
		t.Fatalf("resume threshold must stay below ahead limit: %#v", policy)
	}
	if policy.IdleTTL <= 0 || policy.WorkerIdleTTL <= 0 || policy.KeepBehindSegments <= 0 {
		t.Fatalf("policy defaults not restored: %#v", policy)
	}
}

func TestUHDTo1080CPUScaleUsesFastBilinear(t *testing.T) {
	m := &Manager{}
	filter := m.videoFilter(Spec{Width: 3840, Height: 2160, TargetWidth: 1920, TargetHeight: 1080}, encoderCandidate{name: "libx264", hardware: "cpu"})
	if filter != "scale=1920:1080:flags=fast_bilinear" {
		t.Fatalf("expected cheap UHD compatibility scaler, got %q", filter)
	}
}

func TestNormalDownscaleKeepsBicubicQuality(t *testing.T) {
	m := &Manager{}
	filter := m.videoFilter(Spec{Width: 1920, Height: 1080, TargetWidth: 1280, TargetHeight: 720}, encoderCandidate{name: "libx264", hardware: "cpu"})
	if filter != "scale=1280:720:flags=bicubic" {
		t.Fatalf("expected balanced bicubic scaler outside UHD fallback, got %q", filter)
	}
}
