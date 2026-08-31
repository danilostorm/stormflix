package streaming

import (
	"strings"
	"testing"
)

func TestCPUH264FallbackUsesLiveSuperfastPreset(t *testing.T) {
	args := encoderArgs(encoderCandidate{name: "libx264", hardware: "cpu"}, Spec{TargetBitrateKbps: 8000})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-preset superfast") {
		t.Fatalf("expected low-CPU superfast live preset, got %q", joined)
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
