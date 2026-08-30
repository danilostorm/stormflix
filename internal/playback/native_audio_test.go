package playback

import "testing"

func TestNativeClientKeepsOriginalMultiAudioWhenAnyTrackIsSupported(t *testing.T) {
	req := baseRequest()
	req.ClientKind = "android"
	req.Capabilities.Containers = append(req.Capabilities.Containers, "mkv")
	req.Capabilities.NativeAudioTrackSelection = true
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "dts", Language: "pt-BR", Default: true},
		{Index: 2, Type: "audio", Codec: "aac", Language: "en"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeDirectPlay || !plan.Available || !plan.ClientSelectsAudio || plan.AudioTranscode || plan.VideoTranscode {
		t.Fatalf("expected native multi-audio direct play, got %+v", plan)
	}
	if plan.AudioStream != -1 || plan.AudioCodec != "" || plan.SourceAudioCodec != "" {
		t.Fatalf("native audio selection must not pin the unsupported preferred stream: %+v", plan)
	}
}

func TestNativeClientUsesAACCompatibilityWhenNoAudioTrackIsSupported(t *testing.T) {
	req := baseRequest()
	req.ClientKind = "android"
	req.Capabilities.Containers = append(req.Capabilities.Containers, "mkv")
	req.Capabilities.NativeAudioTrackSelection = true
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "dts", Language: "pt-BR"},
	}}
	plan := DecideForClient(source, req)
	if plan.Mode != ModeAudioCompatibility || !plan.AudioTranscode || plan.VideoTranscode {
		t.Fatalf("expected audio-only compatibility, got %+v", plan)
	}
}

func TestNativeAudioSelectionCannotBypassVideoTranscodePolicy(t *testing.T) {
	req := baseRequest()
	req.ClientKind = "tv"
	req.Capabilities.Containers = append(req.Capabilities.Containers, "mkv")
	req.Capabilities.NativeAudioTrackSelection = true
	req.Capabilities.VideoProfiles = []VideoProfile{{Codec: "h264", MaxWidth: 1920, MaxHeight: 1080}}
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 3840, Height: 2160},
		{Index: 1, Type: "audio", Codec: "dts", Language: "pt-BR"},
		{Index: 2, Type: "audio", Codec: "aac", Language: "eng"},
	}}
	plan := DecideForClient(source, req)
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.VideoTranscode || plan.ClientSelectsAudio || plan.ReasonCode != "video_resolution_unsupported" {
		t.Fatalf("native audio override bypassed video transcode policy: %+v", plan)
	}
	if plan.TargetVideoWidth != 1920 || plan.TargetVideoHeight != 1080 {
		t.Fatalf("expected native client to keep the 1080p transcode cap, got %+v", plan)
	}
}
