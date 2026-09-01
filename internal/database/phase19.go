package database

import (
	"database/sql"
	"fmt"
)

// Phase 19 adds conservative multi-segment credit detection without changing
// the legacy one-marker-per-kind table. Keeping the old table intact preserves
// manual/chapter corrections while automatic analysis can store several safe
// credit intervals separated by real content such as post-credit scenes.
func migratePhase19(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS media_marker_segments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    segment_index INTEGER NOT NULL DEFAULT 0,
    start_seconds REAL NOT NULL,
    end_seconds REAL NOT NULL,
    source TEXT NOT NULL DEFAULT 'automatic',
    confidence REAL NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE,
    CHECK(kind IN ('credits')),
    CHECK(source IN ('manual','chapter','automatic')),
    CHECK(segment_index >= 0),
    CHECK(end_seconds > start_seconds),
    UNIQUE(media_id, kind, segment_index)
);

CREATE INDEX IF NOT EXISTS idx_media_marker_segments_media
    ON media_marker_segments(media_id, kind, start_seconds, end_seconds);

CREATE TABLE IF NOT EXISTS media_credit_analysis (
    media_id INTEGER PRIMARY KEY,
    source_modified_unix INTEGER NOT NULL DEFAULT 0,
    season_size INTEGER NOT NULL DEFAULT 0,
    credit_status TEXT NOT NULL DEFAULT 'pending',
    credit_confidence REAL NOT NULL DEFAULT 0,
    segment_count INTEGER NOT NULL DEFAULT 0,
    analyzed_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE,
    CHECK(credit_status IN ('pending','detected','none','error')),
    CHECK(segment_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_media_credit_analysis_status
    ON media_credit_analysis(credit_status, analyzed_at, media_id);

CREATE TABLE IF NOT EXISTS credit_analysis_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    series_key TEXT NOT NULL DEFAULT '',
    series_title TEXT NOT NULL DEFAULT '',
    season_number INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running',
    progress INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    detected INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE,
    CHECK(status IN ('running','completed','completed_with_errors','error'))
);

CREATE INDEX IF NOT EXISTS idx_credit_analysis_jobs_status
    ON credit_analysis_jobs(status, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_credit_analysis_jobs_season
    ON credit_analysis_jobs(library_id, series_key, season_number, id DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase19 automatic credits: %w", err)
	}
	return migratePhase20(db)
}
