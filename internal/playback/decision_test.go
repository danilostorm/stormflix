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
			AllowVideoTranscode:     true,
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

func TestUnsupportedHEVCFallsBackToH264Transcode(t *testing.T) {
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, baseRequest())
	if plan.Mode != ModeVideoTranscode || !plan.Available || !plan.VideoTranscode || plan.VideoCodec != "h264" || plan.SourceVideoCodec != "hevc" || plan.ReasonCode != "video_codec_unsupported" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestVideoTranscodeCanBeDisabledExplicitly(t *testing.T) {
	req := baseRequest()
	req.Capabilities.AllowVideoTranscode = false
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "hevc"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
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

func TestRemuxContainerWithDTSUsesAACBecauseMP4CannotCopyDTS(t *testing.T) {
	req := baseRequest()
	req.Capabilities.AudioCodecs = append(req.Capabilities.AudioCodecs, "dts")
	source := Source{Container: "mkv", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "dts", Language: "pt-BR"}}}
	plan := Decide(source, req)
	if plan.Mode != ModeAudioCompatibility || !plan.AudioTranscode || plan.VideoTranscode {
		t.Fatalf("expected AAC compatibility for DTS->MP4, got %+v", plan)
	}
}

func TestServerSelectsPreferredNonDefaultAudioForWeb(t *testing.T) {
	req := baseRequest()
	req.Capabilities.ServerSelectsAudio = true
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "aac", Language: "eng", Default: true},
		{Index: 2, Type: "audio", Codec: "aac", Language: "pt-BR"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeRemux || !plan.Available || plan.AudioStream != 2 || plan.AudioTranscode || plan.ReasonCode != "server_audio_track_selection" {
		t.Fatalf("expected explicit preferred-track remux, got %+v", plan)
	}
}

func TestServerSelectedPreferredDTSUsesAACWithoutVideoTranscode(t *testing.T) {
	req := baseRequest()
	req.Capabilities.ServerSelectsAudio = true
	source := Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264"},
		{Index: 1, Type: "audio", Codec: "aac", Language: "eng", Default: true},
		{Index: 2, Type: "audio", Codec: "dts", Language: "pt-BR"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeAudioCompatibility || !plan.AudioTranscode || plan.VideoTranscode || plan.AudioStream != 2 {
		t.Fatalf("expected selected DTS audio-only compatibility, got %+v", plan)
	}
}

func TestDecodeProfileScales4KTo1080p(t *testing.T) {
	req := baseRequest()
	req.Capabilities.VideoProfiles = []VideoProfile{{Codec: "h264", MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 60}}
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Width: 3840, Height: 2160, FrameRate: 30}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.VideoTranscode || plan.TargetVideoWidth != 1920 || plan.TargetVideoHeight != 1080 || plan.ReasonCode != "video_resolution_unsupported" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestKnownUnsupportedHDRUsesToneMapping(t *testing.T) {
	req := baseRequest()
	req.Capabilities.VideoProfiles = []VideoProfile{{Codec: "h264", MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60, HDRKnown: true, HDRTypes: []string{}}}
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: 24, HDR: "hdr10"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.ToneMap || plan.ReasonCode != "video_hdr_unsupported" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestDecodeProfileAllowsAdvertisedHDR(t *testing.T) {
	req := baseRequest()
	req.Capabilities.VideoProfiles = []VideoProfile{{Codec: "h264", MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60, HDRKnown: true, HDRTypes: []string{"hdr10"}}}
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: 24, HDR: "hdr10"}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeDirectPlay {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestDirectPlayBitrateLimitUsesVideoTranscode(t *testing.T) {
	req := baseRequest()
	req.Capabilities.DirectPlayMaxBitrateKbps = 10000
	req.Capabilities.MaxTranscodeBitrateKbps = 8000
	source := Source{Container: "mp4", BitrateKbps: 25000, Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Height: 1080, Width: 1920}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.VideoTranscode || plan.ReasonCode != "direct_play_bitrate_limit" || plan.TargetBitrateKbps != 8000 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestQualityPreferenceCapsTranscodeResolution(t *testing.T) {
	req := baseRequest()
	req.Quality = "720p"
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if plan.TargetVideoHeight != 720 || plan.TargetVideoWidth != 1280 || plan.Quality != "720p" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestExplicitQualityDownshiftTranscodesOtherwiseCompatibleSource(t *testing.T) {
	req := baseRequest()
	req.Quality = "720p"
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080, FrameRate: 24}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeVideoTranscode || !plan.VideoTranscode || plan.ReasonCode != "quality_limit" || plan.TargetVideoWidth != 1280 || plan.TargetVideoHeight != 720 {
		t.Fatalf("expected explicit 720p quality cap to transcode 1080p source, got %+v", plan)
	}
}

func TestOriginalQualityKeepsCompatibleDirectPlay(t *testing.T) {
	req := baseRequest()
	req.Quality = "original"
	source := Source{Container: "mp4", Streams: []Stream{{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080}, {Index: 1, Type: "audio", Codec: "aac"}}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeDirectPlay || plan.VideoTranscode || plan.Quality != "original" {
		t.Fatalf("expected original quality to preserve compatible direct play, got %+v", plan)
	}
}
