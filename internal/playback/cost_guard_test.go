package playback

import "testing"

func TestAutomatic4KIncompatibleSourceFallsBackTo1080p(t *testing.T) {
	req := baseRequest()
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeVideoTranscode || plan.TargetVideoWidth != 1920 || plan.TargetVideoHeight != 1080 || plan.TargetBitrateKbps != 8000 {
		t.Fatalf("expected automatic 4K compatibility transcode to be capped at 1080p, got %+v", plan)
	}
	if !containsNormalized(plan.TranscodeReasons, "auto_4k_cost_guard") {
		t.Fatalf("expected auto_4k_cost_guard reason, got %+v", plan.TranscodeReasons)
	}
}

func TestOriginal4KIncompatibleSourceAlsoFallsBackTo1080p(t *testing.T) {
	req := baseRequest()
	req.Quality = "original"
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeVideoTranscode || plan.TargetVideoWidth != 1920 || plan.TargetVideoHeight != 1080 || plan.TargetBitrateKbps != 8000 {
		t.Fatalf("Original cannot preserve incompatible UHD and should use the safe 1080p compatibility ceiling, got %+v", plan)
	}
}

func TestCompatible4KRemainsDirectPlayAtFullResolution(t *testing.T) {
	req := baseRequest()
	req.Capabilities.VideoCodecs = append(req.Capabilities.VideoCodecs, "hevc")
	req.Capabilities.VideoProfiles = []VideoProfile{{Codec: "hevc", MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}}
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeDirectPlay || !plan.Available || plan.VideoTranscode {
		t.Fatalf("compatible UHD must remain Direct Play, got %+v", plan)
	}
}

func TestExplicit2160pDoesNotTriggerAutomaticCostGuard(t *testing.T) {
	req := baseRequest()
	req.Quality = "2160p"
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeVideoTranscode || plan.TargetVideoHeight != 2160 {
		t.Fatalf("explicit 2160p choice should stay authoritative, got %+v", plan)
	}
	if containsNormalized(plan.TranscodeReasons, "auto_4k_cost_guard") {
		t.Fatalf("automatic guard must not override explicit quality: %+v", plan)
	}
}

func Test4KAudioOnlyCompatibilityKeepsVideoCopy(t *testing.T) {
	req := baseRequest()
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "dts", Language: "pt-BR"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeAudioCompatibility || plan.VideoTranscode || !plan.AudioTranscode {
		t.Fatalf("audio-only incompatibility must never invoke 4K video transcode, got %+v", plan)
	}
}
