package database

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesPhase23GameSavePreviewSupport(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var schema string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='game_saves'`).Scan(&schema); err != nil {
		t.Fatalf("game_saves schema: %v", err)
	}
	if !strings.Contains(schema, "'preview'") {
		t.Fatalf("phase23 preview kind missing from game_saves check: %s", schema)
	}
	for _, name := range []string{"idx_game_saves_profile_updated", "idx_game_saves_game"} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase23 index %s missing: %v", name, err)
		}
	}
}
