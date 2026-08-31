package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase14MovieCollectionColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, column := range []string{"collection_tmdb_id", "collection_name", "collection_checked_at"} {
		if !testColumnExists(t, db, "media_metadata", column) {
			t.Fatalf("phase14 media_metadata column %s missing", column)
		}
	}
	var index string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_metadata_collection'`).Scan(&index); err != nil {
		t.Fatalf("phase14 collection index missing: %v", err)
	}
}
