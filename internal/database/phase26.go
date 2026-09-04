package database

import (
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 26

// Phase 26 persists client-observed startup and stall telemetry. These values
// distinguish server planning time from time-to-first-frame on the device.
func migratePhase26(db *sql.DB) error {
	var alreadyApplied bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=?)`, currentSchemaVersion).Scan(&alreadyApplied); err != nil {
		return fmt.Errorf("check phase26 migration: %w", err)
	}
	columns := []struct{ name, definition string }{
		{"plan_ms", "REAL NOT NULL DEFAULT 0"},
		{"first_frame_ms", "REAL NOT NULL DEFAULT 0"},
		{"startup_ms", "REAL NOT NULL DEFAULT 0"},
		{"stall_count", "INTEGER NOT NULL DEFAULT 0"},
		{"last_stall_ms", "REAL NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := ensureColumn(db, "playback_sessions", column.name, column.definition); err != nil {
			return fmt.Errorf("migrate phase26 %s: %w", column.name, err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migrate phase26 begin: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS catalog_entity_genres (
    entity_key TEXT NOT NULL,
    genre TEXT NOT NULL,
    library_id INTEGER NOT NULL,
    modified_unix INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(entity_key,genre)
);
CREATE INDEX IF NOT EXISTS idx_catalog_entity_genres_popular
    ON catalog_entity_genres(genre COLLATE NOCASE,library_id,modified_unix DESC,entity_key);
CREATE INDEX IF NOT EXISTS idx_catalog_entity_genres_recent
    ON catalog_entity_genres(genre COLLATE NOCASE,modified_unix DESC,entity_key);
`); err != nil {
		return fmt.Errorf("create catalog genre projection: %w", err)
	}
	if !alreadyApplied {
		// Existing installations need one asynchronous projection rebuild to
		// populate the new derived genre table. Fresh databases are still empty.
		if _, err := tx.Exec(`UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE EXISTS(SELECT 1 FROM catalog_entities)`); err != nil {
			return fmt.Errorf("mark catalog genre projection dirty: %w", err)
		}
	}
	if err := recordMigration(tx, currentSchemaVersion, "playback-startup-and-home-telemetry", "phase26-v2"); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", currentSchemaVersion)); err != nil {
		return fmt.Errorf("set sqlite user_version: %w", err)
	}
	return tx.Commit()
}
