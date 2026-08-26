package database

import (
	"database/sql"
	"fmt"
)

// Phase 6 adds content classification, local discovery analytics and profile
// activity without moving StormFlix away from its single-server SQLite model.
func migratePhase6(db *sql.DB) error {
	columns := []struct{ table, name, definition string }{
		{"media_metadata", "content_rating", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "content_rating_age", "INTEGER NOT NULL DEFAULT -1"},
		{"media_metadata", "release_date", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "manual_match", "INTEGER NOT NULL DEFAULT 0"},
		{"profiles", "content_rating_limit", "INTEGER NOT NULL DEFAULT 18"},
	}
	for _, c := range columns {
		if err := ensureColumn(db, c.table, c.name, c.definition); err != nil {
			return err
		}
	}

	const schema = `
CREATE TABLE IF NOT EXISTS profile_watch_daily (
    profile_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    watch_date TEXT NOT NULL,
    seconds_watched REAL NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(profile_id, media_id, watch_date),
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_watch_daily_date_media
    ON profile_watch_daily(watch_date, media_id);
CREATE INDEX IF NOT EXISTS idx_watch_daily_profile_date
    ON profile_watch_daily(profile_id, watch_date);
CREATE INDEX IF NOT EXISTS idx_metadata_release_date
    ON media_metadata(release_date) WHERE release_date <> '';
CREATE INDEX IF NOT EXISTS idx_metadata_content_rating
    ON media_metadata(content_rating_age);
CREATE INDEX IF NOT EXISTS idx_metadata_manual_match
    ON media_metadata(manual_match) WHERE manual_match = 1;
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase6 database: %w", err)
	}

	// Existing kids profiles start conservatively at age 10. Regular profiles
	// remain unrestricted unless the owner explicitly chooses another limit.
	if _, err := db.Exec(`UPDATE profiles SET content_rating_limit=10 WHERE is_kids=1 AND content_rating_limit=18`); err != nil {
		return fmt.Errorf("initialize profile rating limits: %w", err)
	}
	return nil
}
