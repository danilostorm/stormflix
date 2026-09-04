package database

import (
	"database/sql"
	"fmt"
)

const currentSchemaVersion = 26

// Phase 26 persists client-observed startup and stall telemetry. These values
// distinguish server planning time from time-to-first-frame on the device.
func migratePhase26(db *sql.DB) error {
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
	if err := recordMigration(tx, currentSchemaVersion, "playback-startup-telemetry", "phase26-v1"); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", currentSchemaVersion)); err != nil {
		return fmt.Errorf("set sqlite user_version: %w", err)
	}
	return tx.Commit()
}
