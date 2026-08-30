package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlaybackPlanRequestAllowsAdditiveClientCapabilities(t *testing.T) {
	payload := `{
		"client_kind":"android",
		"client_name":"StormFlix Android",
		"client_version":"0.5.2",
		"playback_session_id":"android-test-session",
		"quality":"auto",
		"preferred_audio_language":"pt-BR",
		"future_top_level_field":"ignored",
		"capabilities":{
			"containers":["mp4","mkv"],
			"video_codecs":["h264","hevc"],
			"audio_codecs":["aac","eac3"],
			"video_profiles":[{"codec":"h264","max_width":3840,"max_height":2160,"max_frame_rate":60,"hdr_known":false,"future_profile_field":true}],
			"subtitle_formats":["vtt","srt"],
			"allow_remux":true,
			"allow_audio_compatibility":true,
			"allow_video_transcode":true,
			"max_transcode_bitrate_kbps":16000,
			"native_audio_track_selection":true,
			"server_selects_audio":false,
			"picture_in_picture":true,
			"media_session":true,
			"vendor_decoder_hint":"ignored"
		}
	}`

	var request playbackPlanRequest
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		t.Fatalf("valid Android capability payload should be forward-compatible: %v", err)
	}
	if request.ClientKind != "android" || request.ClientVersion != "0.5.2" {
		t.Fatalf("unexpected decoded request: kind=%q version=%q", request.ClientKind, request.ClientVersion)
	}
	if len(request.Capabilities.VideoProfiles) != 1 || request.Capabilities.VideoProfiles[0].MaxHeight != 2160 {
		t.Fatalf("video capabilities were not decoded: %+v", request.Capabilities.VideoProfiles)
	}
}

func TestPlaybackPlanRequestStillRejectsMalformedJSON(t *testing.T) {
	var request playbackPlanRequest
	decoder := json.NewDecoder(strings.NewReader(`{"client_kind":"android",`))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err == nil {
		t.Fatal("malformed JSON must still fail")
	}
}
