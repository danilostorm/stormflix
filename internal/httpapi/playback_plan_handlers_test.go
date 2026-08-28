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

func TestNormalizePlaybackSessionID(t *testing.T) {
	if got := normalizePlaybackSessionID(" session-1:abc "); got != "session-1:abc" {
		t.Fatalf("unexpected normalized session: %q", got)
	}
	if got := normalizePlaybackSessionID("bad/session"); got != "" {
		t.Fatalf("unsafe session id was accepted: %q", got)
	}
}
