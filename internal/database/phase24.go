package database

import (
	"database/sql"
	"fmt"
)

const migrationLedgerVersion = 24

// Phase 24 establishes the durable migration ledger used by future schema
// changes. Phases 1-23 predate the ledger and are recorded as an audited
// baseline only after their existing idempotent migrations have succeeded.
func migratePhase24(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migrate phase24 begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);`); err != nil {
		return fmt.Errorf("create schema migration ledger: %w", err)
	}

	legacy := []string{
		"base", "metadata", "profiles", "library-sources", "catalog-indexes",
		"discovery", "music", "episodic-categories", "series-identity",
		"catalog-hierarchy", "principal-series", "operational-queues",
		"catalog-automation", "movie-collections", "home-and-trakt",
		"playback-preferences", "marker-analysis", "marker-queue",
		"credits-analysis", "games-catalog", "games-saves",
		"games-metadata", "game-save-previews",
	}
	for i, name := range legacy {
		if err := recordMigration(tx, i+1, name, "legacy-baseline-v1"); err != nil {
			return err
		}
	}
	if err := recordMigration(tx, migrationLedgerVersion, "migration-ledger", "phase24-v1"); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", migrationLedgerVersion)); err != nil {
		return fmt.Errorf("set sqlite user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate phase24 commit: %w", err)
	}
	return migratePhase25(db)
}

func recordMigration(tx *sql.Tx, version int, name, checksum string) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_migrations(version,name,checksum) VALUES(?,?,?)`, version, name, checksum); err != nil {
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	var storedName, storedChecksum string
	if err := tx.QueryRow(`SELECT name,checksum FROM schema_migrations WHERE version=?`, version).Scan(&storedName, &storedChecksum); err != nil {
		return fmt.Errorf("verify migration %d: %w", version, err)
	}
	if storedName != name || storedChecksum != checksum {
		return fmt.Errorf("migration %d checksum mismatch: database has %q/%q, binary expects %q/%q", version, storedName, storedChecksum, name, checksum)
	}
	return nil
}
