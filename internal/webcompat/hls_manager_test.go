package webcompat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHLSPolicy() HLSPolicy {
	p := DefaultHLSPolicy()
	p.MaxBytes = 64 << 20
	p.MinFreeBytes = 0
	p.MinFreePercent = 0
	p.IdleTTL = time.Hour
	p.CleanupInterval = time.Hour
	p.SegmentDuration = 6 * time.Second
	p.BatchSegments = 4
	return p
}

func testHLSSpec() HLSSpec {
	return HLSSpec{
		VideoStream:       0,
		AudioStream:       1,
		VideoCodec:        "h264",
		AudioCodec:        "aac",
		SourceAudioCodec:  "aac",
		DurationSeconds:   61,
		SourceBitrateKbps: 8000,
	}
}

func TestHLSPlaylistUsesBoundedBatchMaps(t *testing.T) {
	m, err := NewHLSManager(t.TempDir(), testHLSPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("session-1", 7, 42, "/media/movie.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	playlist, err := m.Playlist(7, 42, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"/api/v1/media/42/hls/session-1/init/0.mp4",
		"/api/v1/media/42/hls/session-1/init/4.mp4",
		"/api/v1/media/42/hls/session-1/segment/0.m4s",
		"/api/v1/media/42/hls/session-1/segment/10.m4s",
		"#EXT-X-DISCONTINUITY",
		"#EXT-X-ENDLIST",
	} {
		if !strings.Contains(playlist, want) {
			t.Fatalf("playlist missing %q:\n%s", want, playlist)
		}
	}
	if got := strings.Count(playlist, "#EXTINF:"); got != 11 {
		t.Fatalf("expected 11 segments, got %d", got)
	}
}

func TestHLSCloseSessionDeletesUserCacheImmediately(t *testing.T) {
	root := t.TempDir()
	m, err := NewHLSManager(root, testHLSPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("session-close", 9, 77, "/media/movie.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "session-close", "seg-000000.m4s")
	if err := os.WriteFile(path, []byte("fragment"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.CloseSession(9, "session-close") {
		t.Fatal("expected session close to be accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "session-close")); !os.IsNotExist(err) {
		t.Fatalf("session cache must be deleted immediately, stat err=%v", err)
	}
	if m.CloseSession(9, "session-close") {
		t.Fatal("closed session should not be closed twice")
	}
}

func TestHLSCloseCannotDeleteAnotherUsersSession(t *testing.T) {
	root := t.TempDir()
	m, err := NewHLSManager(root, testHLSPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("owned", 10, 1, "/media/movie.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	if m.CloseSession(11, "owned") {
		t.Fatal("another user must not be able to close the session")
	}
	if _, err := os.Stat(filepath.Join(root, "owned")); err != nil {
		t.Fatalf("owner cache unexpectedly removed: %v", err)
	}
}

func TestHLSReplacingSourceClearsPreviousFragments(t *testing.T) {
	root := t.TempDir()
	m, err := NewHLSManager(root, testHLSPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("switch", 3, 100, "/media/a.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "switch", "seg-000000.m4s")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("switch", 3, 101, "/media/b.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old source fragment should be removed on source switch, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "switch")); err != nil {
		t.Fatalf("replacement session directory missing: %v", err)
	}
}

func TestHLSIdleSessionCleanup(t *testing.T) {
	root := t.TempDir()
	p := testHLSPolicy()
	p.IdleTTL = 5 * time.Millisecond
	m, err := NewHLSManager(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("idle", 1, 1, "/media/movie.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if err := m.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "idle")); !os.IsNotExist(err) {
		t.Fatalf("idle session should be removed, stat err=%v", err)
	}
}

func TestHLSGlobalBudgetEvictsDisposableSegmentsBeforeNewBatch(t *testing.T) {
	root := t.TempDir()
	p := testHLSPolicy()
	p.MaxBytes = 100
	p.EvictionTargetPct = 80
	m, err := NewHLSManager(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("budget", 1, 1, "/media/movie.mkv", testHLSSpec()); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "budget", "seg-000000.m4s")
	if err := os.WriteFile(old, make([]byte, 80), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := m.cleanupPressure(40); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old segment should be evicted to reserve the next batch, stat err=%v", err)
	}
}

func TestHLSRejectsUnsafeSessionID(t *testing.T) {
	m, err := NewHLSManager(t.TempDir(), testHLSPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PrepareSession("../escape", 1, 1, "/media/movie.mkv", testHLSSpec()); err == nil {
		t.Fatal("expected unsafe session id to be rejected")
	}
}

func TestNeedsHLSAAC(t *testing.T) {
	for _, codec := range []string{"aac", "ac3", "eac3", "mp3"} {
		if NeedsHLSAAC(codec, false) {
			t.Fatalf("%s should remain stream-copy in fMP4 HLS", codec)
		}
	}
	for _, codec := range []string{"dts", "truehd", "opus", "flac"} {
		if !NeedsHLSAAC(codec, false) {
			t.Fatalf("%s should use AAC for fMP4 HLS compatibility", codec)
		}
	}
	if !NeedsHLSAAC("aac", true) {
		t.Fatal("explicit audio compatibility must force AAC")
	}
}
