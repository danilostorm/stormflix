package playback

import "testing"

func localDecodeRequest() Request {
	req := baseRequest()
	req.LocalDecode = ClientCapabilities{
		Kind:                "web",
		Enabled:             true,
		WASM:                true,
		Worker:              true,
		WebCodecs:           true,
		SecureContext:       true,
		HEVCWASM:            true,
		MaxWidth:            3840,
		MaxHeight:           2160,
		HardwareConcurrency: 12,
	}
	return req
}

func localOriginRequest() Request {
	req := localDecodeRequest()
	req.LocalDecode.OriginalFile = true
	req.LocalDecode.WASMSIMD = true
	req.LocalDecode.WebGL = true
	req.LocalDecode.Containers = []string{"mkv", "mp4", "webm"}
	req.LocalDecode.Codecs = []string{"h264", "hevc", "av1"}
	req.LocalDecode.AudioCodecs = []string{"aac", "ac3", "eac3", "dts", "mp3", "opus", "flac", "vorbis"}
	req.LocalDecode.SubtitleFormats = []string{"vtt", "srt", "ass", "ssa"}
	return req
}

func TestLocalOriginDemuxesMKVWithoutServerFFmpeg(t *testing.T) {
	req := localOriginRequest()
	source := Source{Container: "mkv", BitrateKbps: 18000, Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24},
		{Index: 2, Type: "audio", Codec: "dts", Language: "pt-BR"},
	}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeLocalDecode || !plan.LocalOrigin || plan.Transport != "original_range" || plan.AudioTranscode || plan.VideoTranscode || plan.Container != "mkv" || plan.LocalDecodeEngine != "libmedia-1.3.1" {
		t.Fatalf("expected original-range local playback, got %+v", plan)
	}
}

func TestLocalOriginCanSelectNonDefaultAudioOnClient(t *testing.T) {
	req := localOriginRequest()
	req.Capabilities.ServerSelectsAudio = true
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Language: "eng", Default: true},
		{Index: 3, Type: "audio", Codec: "ac3", Language: "pt-BR"},
	}}
	plan := Decide(source, req)
	if !plan.LocalOrigin || !plan.ClientSelectsAudio || plan.AudioStream != 3 || plan.AudioTranscode {
		t.Fatalf("expected client-side audio selection without FFmpeg, got %+v", plan)
	}
}

func TestLocalOriginFallsBackWhenSIMDIsUnavailable(t *testing.T) {
	req := localOriginRequest()
	req.LocalDecode.WASMSIMD = false
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.LocalOrigin || plan.LocalDecodeEngine != "stormflix-v6-wasm" {
		t.Fatalf("expected transparent v6 fallback, got %+v", plan)
	}
}

func TestLocalOriginHDRStaysOnConservativeServerFallback(t *testing.T) {
	req := localOriginRequest()
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, HDR: "hdr10"},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.LocalOrigin || plan.Mode != ModeVideoTranscode {
		t.Fatalf("HDR must stay on the verified fallback until local tone mapping is certified, got %+v", plan)
	}
}

func TestHEVCUsesLocalDecodeBeforeServerVideoTranscode(t *testing.T) {
	req := localDecodeRequest()
	source := Source{Container: "mkv", BitrateKbps: 18000, Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeLocalDecode || !plan.LocalDecode || plan.VideoTranscode || plan.LocalDecodeCodec != "hevc" {
		t.Fatalf("expected HEVC local decode, got %+v", plan)
	}
}

func TestUltrawide4KHEVCUsesAutomaticLocalDecodeBeforeServerVideoTranscode(t *testing.T) {
	req := localDecodeRequest()
	source := Source{Container: "mkv", BitrateKbps: 24000, Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 1600, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if !plan.Available || plan.Mode != ModeLocalDecode || !plan.LocalDecode || plan.VideoTranscode || plan.VideoWidth != 3840 || plan.VideoHeight != 1600 {
		t.Fatalf("expected 3840x1600 HEVC to use client-local decode, got %+v", plan)
	}
}

func TestLocalDecodeKeepsVideoCopyButCanConvertAudio(t *testing.T) {
	req := localDecodeRequest()
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24},
		{Index: 2, Type: "audio", Codec: "dts", Language: "pt-BR"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeLocalDecode || !plan.LocalDecode || plan.VideoTranscode || !plan.AudioTranscode || plan.AudioCodec != "aac" {
		t.Fatalf("expected local video decode + AAC-only server compatibility, got %+v", plan)
	}
}

func TestExplicitLowerQualityStillUsesServerTranscode(t *testing.T) {
	req := localDecodeRequest()
	req.Quality = "720p"
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeVideoTranscode || !plan.VideoTranscode || plan.LocalDecode || plan.TargetVideoHeight != 720 {
		t.Fatalf("expected requested lower quality to remain server transcode, got %+v", plan)
	}
}

func TestHDRFallsBackToServerUntilClientExplicitlyAdvertisesHDRLocalDecode(t *testing.T) {
	req := localDecodeRequest()
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24, HDR: "hdr10"},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeVideoTranscode || plan.LocalDecode {
		t.Fatalf("expected conservative HDR server fallback, got %+v", plan)
	}
}

func TestLocalDecodeRespectsClientResolutionBudget(t *testing.T) {
	req := localDecodeRequest()
	req.LocalDecode.MaxHeight = 1080
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 3840, Height: 2160, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeVideoTranscode || plan.LocalDecode {
		t.Fatalf("expected device-limit server fallback, got %+v", plan)
	}
}

func TestNonWebClientsDoNotEnterBrowserLocalDecode(t *testing.T) {
	req := localDecodeRequest()
	req.LocalDecode.Kind = "android"
	source := Source{Container: "mkv", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "hevc", Width: 1920, Height: 1080, FrameRate: 24},
		{Index: 1, Type: "audio", Codec: "aac"},
	}}
	plan := Decide(source, req)
	if plan.Mode != ModeVideoTranscode || plan.LocalDecode {
		t.Fatalf("expected Android to keep native/server route, got %+v", plan)
	}
}
