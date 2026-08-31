package httpapi

import "testing"

func TestNormalizePlaybackPreferences(t *testing.T) {
	got := normalizePlaybackPreferences(playbackPreferenceState{
		SkipMode:                  "AUTOMATIC",
		RewindSeconds:             15,
		StillWatching:             true,
		StillWatchingEpisodeLimit: 99,
		StillWatchingHours:        0,
		AutoplayCountdown:         30,
	})
	if got.SkipMode != "automatic" || got.RewindSeconds != 15 || got.AutoplayCountdown != 30 {
		t.Fatalf("unexpected normalized preferences: %+v", got)
	}
	if got.StillWatchingEpisodeLimit != 12 || got.StillWatchingHours != 1 {
		t.Fatalf("expected bounded still-watching thresholds, got %+v", got)
	}
}

func TestNormalizeMarkerSourceManualWinsConfidence(t *testing.T) {
	if source, confidence := normalizeMarkerSource("manual"); source != "manual" || confidence != 1 {
		t.Fatalf("manual marker source mismatch: %s %.2f", source, confidence)
	}
	if source, confidence := normalizeMarkerSource("chapter"); source != "chapter" || confidence >= 1 {
		t.Fatalf("chapter marker confidence should remain below manual: %s %.2f", source, confidence)
	}
}
