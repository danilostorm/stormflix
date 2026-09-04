package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPlaybackStartupTelemetry(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, column := range []string{"plan_ms", "first_frame_ms", "startup_ms", "stall_count", "last_stall_ms"} {
		if !testColumnExists(t, db, "playback_sessions", column) {
			t.Fatalf("phase26 playback_sessions column %s missing", column)
		}
	}
}
