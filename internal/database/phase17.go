package database

import (
	"database/sql"
	"fmt"
)

// Phase 17 tracks automatic intro analysis independently from the marker itself.
// This lets StormFlix avoid re-decoding unchanged episodes, invalidate a whole
// season when its episode count changes, and retry transient ffmpeg/rclone errors
// without ever replacing a manual correction.
func migratePhase17(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS media_marker_analysis (
    media_id INTEGER PRIMARY KEY,
    source_modified_unix INTEGER NOT NULL DEFAULT 0,
    season_size INTEGER NOT NULL DEFAULT 0,
    intro_status TEXT NOT NULL DEFAULT 'pending',
    intro_confidence REAL NOT NULL DEFAULT 0,
    analyzed_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE,
    CHECK(intro_status IN ('pending','detected','none','error'))
);

CREATE INDEX IF NOT EXISTS idx_media_marker_analysis_status
    ON media_marker_analysis(intro_status, analyzed_at, media_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase17 automatic intro analysis: %w", err)
	}
	return nil
}
