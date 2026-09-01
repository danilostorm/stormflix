package httpapi

import (
	"strings"
	"testing"
)

func TestPlaybackGrantAllowedPathIsMediaScoped(t *testing.T) {
	allowed := []string{
		"/api/v1/media/42/stream",
		"/api/v1/media/42/remux",
		"/api/v1/media/42/hls/session/index.m3u8",
		"/api/v1/media/42/webstream/ws_session/index.m3u8",
		"/api/v1/media/42/subtitles/3/vtt",
	}
	for _, path := range allowed {
		if !playbackGrantAllowedPath(42, path) {
			t.Fatalf("expected path to be allowed: %s", path)
		}
	}
	denied := []string{
		"/api/v1/media/43/stream",
		"/api/v1/media/42",
		"/api/v1/media/42/playback/plan",
		"/api/v1/admin/settings",
		"/api/v1/games/42/rom",
	}
	for _, path := range denied {
		if playbackGrantAllowedPath(42, path) {
			t.Fatalf("expected path to be denied: %s", path)
		}
	}
}

func TestRewritePlaylistWithPlaybackGrant(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-MAP:URI=\"init/0.mp4\"\n#EXTINF:4,\nsegment/0.m4s\n"
	got := rewritePlaylistWithPlaybackGrant(input, "abc.DEF")
	if !strings.Contains(got, `URI="init/0.mp4?st=abc.DEF"`) {
		t.Fatalf("init segment did not receive grant: %s", got)
	}
	if !strings.Contains(got, "segment/0.m4s?st=abc.DEF") {
		t.Fatalf("media segment did not receive grant: %s", got)
	}
	if strings.Contains(got, "#EXTINF:4,?st=") {
		t.Fatalf("HLS directives must not be rewritten: %s", got)
	}
}

func TestPlaybackGrantMediaID(t *testing.T) {
	if got := playbackGrantMediaID("/api/v1/media/987/hls/x/index.m3u8"); got != 987 {
		t.Fatalf("media id = %d, want 987", got)
	}
	if got := playbackGrantMediaID("/api/v1/games/987/rom"); got != 0 {
		t.Fatalf("unexpected media id: %d", got)
	}
}
