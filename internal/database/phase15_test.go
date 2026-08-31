package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase15ProfileTraktAndHomeIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"profile_trakt", "profile_trakt_device_auth"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("phase15 table %s missing: %v", table, err)
		}
	}
	for _, index := range []string{
		"idx_media_artwork_selected_lookup",
		"idx_media_available_library_title",
		"idx_media_modified_available",
		"idx_metadata_movie_collection_backfill",
		"idx_metadata_collection_browse",
	} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, index).Scan(&name); err != nil {
			t.Fatalf("phase15 index %s missing: %v", index, err)
		}
	}
}
