package database

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesPhase23GameSavePreviewSupport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stormflix.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

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
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	// Migrations are evaluated on every server start. Phase 23 must recognize
	// the already-upgraded CHECK constraint and become a no-op on reopen.
	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen upgraded database: %v", err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='game_saves'`).Scan(&schema); err != nil {
		t.Fatalf("game_saves after reopen: %v", err)
	}
	if !strings.Contains(schema, "'preview'") {
		t.Fatal("phase23 preview support disappeared after reopen")
	}
}
