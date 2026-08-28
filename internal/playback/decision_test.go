package playback

import "testing"

func baseRequest() Request {
	return Request{
		ClientKind: "web",
		Capabilities: Capabilities{
			Containers:              []string{"mp4", "webm"},
			VideoCodecs:             []string{"h264", "vp9", "av1"},
			AudioCodecs:             []string{"aac", "mp3", "opus"},
			AllowRemux:              true,
			AllowAudioCompatibility: true,
		},
		PreferredAudioLanguage: "pt-BR",
	}
}

func TestDecideDirectPlayH264AACMP4(t *testing.T) {
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac", Language: "pt-BR"}}}
	plan := Decide(source, baseRequest())
	if plan.Mode != ModeDirectPlay || !plan.Available || plan.VideoTranscode || plan.AudioTranscode {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestDecideH264DTSUsesAudioOnlyCompatibility(t *testing.T) {
	source := Source{Container: "mkv", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 2, Type: "audio", Codec: "dts", Language: "pt-BR"}}}
	plan := Decide(source, baseRequest())
	if plan.Mode != ModeAudioCompatibility || !plan.AudioTranscode || plan.VideoTranscode || plan.AudioCodec != "aac" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestDecideUnsupportedHEVCNeverSilentlyTranscodesVideo(t *testing.T) {
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "hevc"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, baseRequest())
	if plan.Mode != ModeUnsupported || plan.Available || plan.VideoTranscode || plan.ReasonCode != "video_codec_unsupported" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestDecideAndroidHEVCDirectPlay(t *testing.T) {
	req := baseRequest()
	req.ClientKind = "android"
	req.Capabilities.VideoCodecs = append(req.Capabilities.VideoCodecs, "hevc")
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "hevc"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if plan.Mode != ModeDirectPlay || !plan.Available {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestPreferredPortugueseFallbackOrder(t *testing.T) {
	req := baseRequest()
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "aac", Language: "por"},
		{Index: 2, Type: "audio", Codec: "aac", Language: "pt"},
	}}
	plan := Decide(source, req)
	if plan.AudioStream != 2 {
		t.Fatalf("expected pt before por, got %+v", plan)
	}

	source.Streams = []Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac", Language: "por"}}
	plan = Decide(source, req)
	if plan.AudioStream != 1 {
		t.Fatalf("expected por fallback, got %+v", plan)
	}
}

func TestRemuxWhenOnlyContainerIsUnsupported(t *testing.T) {
	source := Source{Container: "mkv", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "aac", Language: "pt-BR"}}}
	plan := Decide(source, baseRequest())
	if plan.Mode != ModeRemux || !plan.Available || plan.AudioTranscode || plan.VideoTranscode || plan.Container != "mp4" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}
