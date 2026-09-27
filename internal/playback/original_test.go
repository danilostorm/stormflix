package playback

import "testing"

func originalTestSource() Source {
	return Source{Container: "mp4", Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Default: true},
	}}
}

func originalTestRequest() Request {
	return Request{OriginalOnly: true, Quality: "480p", ClientKind: "web", Capabilities: Capabilities{
		Containers: []string{"mp4"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"},
		AllowRemux: true, AllowAudioCompatibility: true, AllowVideoTranscode: true,
	}, LocalDecode: ClientCapabilities{Kind: "web", Enabled: true, OriginalFile: true,
		WASM: true, WASMSIMD: true, Worker: true, WebGL: true, SecureContext: true,
		HEVCWASM: true, AV1WASM: true, WebCodecs: true,
		Containers: []string{"mp4", "mkv"}, Codecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac", "dts"},
		MaxHeight: 1080, MaxWidth: 1920}}
}

func TestOriginalPlaybackNeverConvertsEvenWithOldClientCapabilities(t *testing.T) {
	for _, tc := range []struct{ name, container, video, audio, mode string }{
		{"native", "mp4", "h264", "aac", ModeDirectPlay},
		{"local container and audio", "mkv", "hevc", "dts", ModeLocalDecode},
		{"unsupported codec", "mp4", "wmv3", "aac", ModeUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := originalTestSource()
			source.Container, source.Streams[0].Codec, source.Streams[1].Codec = tc.container, tc.video, tc.audio
			plan := DecideForClient(source, originalTestRequest())
			if plan.Mode != tc.mode || plan.VideoTranscode || plan.AudioTranscode || plan.Quality != "original" || !plan.OriginalOnly {
				t.Fatalf("unexpected original plan: %+v", plan)
			}
			if plan.Mode == ModeLocalDecode && (!plan.LocalOrigin || plan.Transport != "original_range") {
				t.Fatalf("local decoder requested a server conversion: %+v", plan)
			}
		})
	}
}

func TestRuntimeRejectionAndAudioSelectionUseLocalOriginal(t *testing.T) {
	source, request := originalTestSource(), originalTestRequest()
	request.NativeSourceRejected = true
	if plan := Decide(source, request); !plan.Available || !plan.LocalOrigin {
		t.Fatalf("runtime rejection did not use local original: %+v", plan)
	}
	request.LocalDecode.OriginalFile = false
	if plan := Decide(source, request); plan.Available {
		t.Fatalf("runtime failure must not retry the rejected native source: %+v", plan)
	}
	request = originalTestRequest()
	request.Capabilities.ServerSelectsAudio = true
	source.Streams = append(source.Streams, Stream{Index: 2, Type: "audio", Codec: "aac", Language: "pt"})
	plan := ApplyAudioStream(source, request, Decide(source, request), 2)
	if !plan.Available || !plan.LocalOrigin || plan.AudioStream != 2 || plan.AudioTranscode {
		t.Fatalf("non-default audio should be selected locally: %+v", plan)
	}
}

func TestOriginalLocalDecoderOnLANAndMobile(t *testing.T) {
	for _, kind := range []string{"web", "desktop", "mobile_web", "android_webview"} {
		t.Run(kind, func(t *testing.T) {
			r := originalTestRequest()
			r.NativeSourceRejected = true
			r.LocalDecode.Kind = kind
			r.LocalDecode.SecureContext = false // ScriptProcessor audio fallback on LAN.
			plan := Decide(originalTestSource(), r)
			if !plan.Available || !plan.LocalOrigin || plan.AudioTranscode || plan.VideoTranscode {
				t.Fatalf("capable LAN/mobile decoder was blocked: %+v", plan)
			}
			r.LocalDecode.WASMSIMD = false
			if Decide(originalTestSource(), r).Available {
				t.Fatal("missing required decoder feature must still reject the route")
			}
		})
	}
}
