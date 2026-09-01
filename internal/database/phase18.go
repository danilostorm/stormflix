package database

import (
	"database/sql"
	"fmt"
)

// Phase 18 makes automatic intro detection observable from the same persistent
// operational queue used by library scans and metadata jobs.
func migratePhase18(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS marker_analysis_jobs (
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

CREATE INDEX IF NOT EXISTS idx_marker_analysis_jobs_status
    ON marker_analysis_jobs(status, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_marker_analysis_jobs_season
    ON marker_analysis_jobs(library_id, series_key, season_number, id DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase18 observable marker jobs: %w", err)
	}
	return nil
}
