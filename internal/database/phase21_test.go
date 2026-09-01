package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase21GameSaveState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, name := range []string{
		"game_saves", "idx_game_saves_profile_updated", "idx_game_saves_game",
		"game_play_sessions", "idx_game_play_sessions_profile", "idx_game_play_sessions_seen",
	} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase21 object %s missing: %v", name, err)
		}
	}
}
