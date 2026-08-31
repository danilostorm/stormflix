package database

import (
	"database/sql"
	"fmt"
)

// Phase 14 stores TMDB movie-franchise membership used by the automatic
// Collections menu. Membership is presentation metadata only: physical media,
// libraries and playback identity remain unchanged.
func migratePhase14(db *sql.DB) error {
	for _, column := range []struct {
		table, name, definition string
	}{
		{"media_metadata", "collection_tmdb_id", "INTEGER NOT NULL DEFAULT 0"},
		{"media_metadata", "collection_name", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "collection_source_tmdb_id", "INTEGER NOT NULL DEFAULT 0"},
		{"media_metadata", "collection_checked_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_metadata_collection ON media_metadata(collection_tmdb_id,collection_name,media_id)`); err != nil {
		return fmt.Errorf("migrate phase14 movie collections: %w", err)
	}
	return migratePhase15(db)
}
