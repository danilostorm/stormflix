package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danilostorm/stormflix/internal/webcompat"
)

func TestAndroidUserAgentDoesNotForceAAC(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://stormflix.test/api/v1/media/1/compatibility", nil)
	req.Header.Set("User-Agent", "StormFlix-Android/0.2.3")
	plan := webcompat.Plan{
		Available:        true,
		AudioStream:      2,
		AudioCodec:       "eac3",
		SourceAudioCodec: "eac3",
	}

	preferAACForPlayback(req, &plan)

	if plan.AudioTranscode {
		t.Fatal("Android User-Agent alone must not force audio transcoding")
	}
	if plan.AudioCodec != "eac3" {
		t.Fatalf("AudioCodec=%q, want original eac3", plan.AudioCodec)
	}
}

func TestExplicitAACFallbackForcesAAC(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://stormflix.test/api/v1/media/1/remux?audio=aac", nil)
	plan := webcompat.Plan{
		Available:        true,
		AudioStream:      2,
		AudioCodec:       "eac3",
		SourceAudioCodec: "eac3",
	}

	preferAACForPlayback(req, &plan)

	if !plan.AudioTranscode {
		t.Fatal("explicit audio=aac fallback must enable audio transcoding")
	}
	if plan.AudioCodec != "aac" {
		t.Fatalf("AudioCodec=%q, want aac", plan.AudioCodec)
	}
}
