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
