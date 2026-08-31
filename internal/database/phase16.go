package database

import (
	"database/sql"
	"fmt"
)

// Phase 16 moves Playback Delight preferences and skip markers from a
// device-local prototype to shared server state. The tables are deliberately
// independent from playback transport/cache tables so no media bytes or live
// transcode state are involved.
func migratePhase16(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS profile_playback_preferences (
    profile_id INTEGER PRIMARY KEY,
    skip_mode TEXT NOT NULL DEFAULT 'manual',
    rewind_seconds INTEGER NOT NULL DEFAULT 10,
    still_watching INTEGER NOT NULL DEFAULT 1,
    still_watching_episode_limit INTEGER NOT NULL DEFAULT 3,
    still_watching_hours INTEGER NOT NULL DEFAULT 3,
    autoplay_countdown INTEGER NOT NULL DEFAULT 10,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_markers (
    media_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    start_seconds REAL NOT NULL,
    end_seconds REAL NOT NULL,
    source TEXT NOT NULL DEFAULT 'manual',
    confidence REAL NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(media_id, kind),
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE,
    CHECK(kind IN ('intro','credits','recap')),
    CHECK(end_seconds > start_seconds)
);

CREATE INDEX IF NOT EXISTS idx_media_markers_media
    ON media_markers(media_id, start_seconds, end_seconds);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase16 playback delight state: %w", err)
	}
	return nil
}
