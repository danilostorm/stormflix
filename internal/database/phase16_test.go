package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase16PlaybackDelightState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"profile_playback_preferences", "media_markers"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("phase16 table %s missing: %v", table, err)
		}
	}
	var index string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_media_markers_media'`).Scan(&index); err != nil {
		t.Fatalf("phase16 media marker index missing: %v", err)
	}
}
