package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase22GamesAdminMetadata(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, name := range []string{
		"game_metadata", "idx_game_metadata_provider", "idx_game_metadata_locked",
		"game_provider_settings",
		"game_metadata_jobs", "idx_game_metadata_jobs_status", "idx_game_metadata_jobs_library",
	} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase22 object %s missing: %v", name, err)
		}
	}
}
