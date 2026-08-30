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
