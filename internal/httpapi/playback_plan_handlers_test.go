package httpapi

import (
	"strings"
	"testing"

	"github.com/danilostorm/stormflix/internal/playback"
)

func TestPlaybackExecutionURLCarriesSelectedAudioStream(t *testing.T) {
	mediaURL, prepareURL := playbackExecutionURLs(42, playback.Plan{Mode: playback.ModeAudioCompatibility, AudioStream: 7})
	if !strings.Contains(mediaURL, "audio=aac") || !strings.Contains(mediaURL, "audio_stream=7") {
		t.Fatalf("unexpected media URL: %s", mediaURL)
	}
	if !strings.Contains(prepareURL, "audio=aac") || !strings.Contains(prepareURL, "audio_stream=7") {
		t.Fatalf("unexpected prepare URL: %s", prepareURL)
	}
}

func TestPlaybackExecutionDirectPlayHasNoCompatibilityQuery(t *testing.T) {
	mediaURL, prepareURL := playbackExecutionURLs(8, playback.Plan{Mode: playback.ModeDirectPlay, AudioStream: -1})
	if mediaURL != "/api/v1/media/8/stream" || prepareURL != "" {
		t.Fatalf("unexpected direct play URLs: media=%q prepare=%q", mediaURL, prepareURL)
	}
}

func TestPlaybackExecutionLocalOriginUsesOriginalRangeURL(t *testing.T) {
	mediaURL, prepareURL := playbackExecutionURLs(9, playback.Plan{Mode: playback.ModeLocalDecode, LocalOrigin: true, Transport: "original_range", AudioStream: 4})
	// The web plan handler deliberately normalizes this to the authenticated
	// source endpoint; no compatibility query or prepare job may be introduced.
	if strings.Contains(mediaURL, "audio=") || strings.Contains(mediaURL, "prepare") || strings.Contains(prepareURL, "prepare") {
		t.Fatalf("local-origin route must not request server processing: media=%q prepare=%q", mediaURL, prepareURL)
	}
	if !webPlanUsesOriginalTransport(playback.Plan{Mode: playback.ModeLocalDecode, LocalOrigin: true, Transport: "original_range"}) {
		t.Fatal("local-origin plan would fall through to the FFmpeg web-session branch")
	}
	if webPlanUsesOriginalTransport(playback.Plan{Mode: playback.ModeLocalDecode, LocalOrigin: true}) {
		t.Fatal("an incomplete local-decode plan must not bypass the compatibility engine")
	}
}

func TestNativeStormFlixClientsUseDynamicHLSCompatibility(t *testing.T) {
	for _, kind := range []string{"web", "android", "tv", "android_tv", "firetv", "fire_tv", " Android "} {
		if !clientUsesDynamicHLS(kind) {
			t.Fatalf("expected %q to use dynamic HLS compatibility", kind)
		}
	}
	for _, kind := range []string{"", "unknown", "legacy", "jellyfin"} {
		if clientUsesDynamicHLS(kind) {
			t.Fatalf("did not expect %q to use native StormFlix dynamic HLS path", kind)
		}
	}
}

func TestNormalizePlaybackSessionID(t *testing.T) {
	if got := normalizePlaybackSessionID(" session-1:abc "); got != "session-1:abc" {
		t.Fatalf("unexpected normalized session: %q", got)
	}
	if got := normalizePlaybackSessionID("bad/session"); got != "" {
		t.Fatalf("unsafe session id was accepted: %q", got)
	}
}
