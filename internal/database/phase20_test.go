package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase20GamesCatalog(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, name := range []string{
		"games", "idx_games_library_platform_title", "idx_games_hash",
		"game_files", "idx_game_files_available", "idx_game_files_path",
		"game_profile_state", "idx_game_profile_recent", "idx_game_profile_favorite",
		"game_scan_jobs", "idx_game_scan_jobs_status", "idx_game_scan_jobs_library",
	} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase20 object %s missing: %v", name, err)
		}
	}
}
