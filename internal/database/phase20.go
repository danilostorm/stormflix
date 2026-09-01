package database

import (
	"database/sql"
	"fmt"
)

// Phase 20 introduces Games as a first-class StormFlix catalog. Game-specific
// identity/state intentionally lives outside video media so ROMs cannot leak
// into movie/series metadata or playback pipelines.
func migratePhase20(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    platform TEXT NOT NULL,
    title TEXT NOT NULL,
    sort_title TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    cover_path TEXT NOT NULL DEFAULT '',
    metadata_provider TEXT NOT NULL DEFAULT '',
    metadata_id TEXT NOT NULL DEFAULT '',
    overview TEXT NOT NULL DEFAULT '',
    release_year INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE,
    UNIQUE(library_id, platform, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_games_library_platform_title
    ON games(library_id, platform, sort_title, title, id);
CREATE INDEX IF NOT EXISTS idx_games_hash
    ON games(content_hash, platform);

CREATE TABLE IF NOT EXISTS game_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    extension TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    modified_unix INTEGER NOT NULL DEFAULT 0,
    available INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(game_id) REFERENCES games(id) ON DELETE CASCADE,
    UNIQUE(game_id, path)
);

CREATE INDEX IF NOT EXISTS idx_game_files_available
    ON game_files(game_id, available, path);
CREATE INDEX IF NOT EXISTS idx_game_files_path
    ON game_files(path);

CREATE TABLE IF NOT EXISTS game_profile_state (
    profile_id INTEGER NOT NULL,
    game_id INTEGER NOT NULL,
    favorite INTEGER NOT NULL DEFAULT 0,
    play_seconds INTEGER NOT NULL DEFAULT 0,
    last_played_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(profile_id, game_id),
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(game_id) REFERENCES games(id) ON DELETE CASCADE,
    CHECK(favorite IN (0,1)),
    CHECK(play_seconds >= 0)
);

CREATE INDEX IF NOT EXISTS idx_game_profile_recent
    ON game_profile_state(profile_id, last_played_at DESC, game_id);
CREATE INDEX IF NOT EXISTS idx_game_profile_favorite
    ON game_profile_state(profile_id, favorite, updated_at DESC, game_id);

CREATE TABLE IF NOT EXISTS game_scan_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    progress INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    matched INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE,
    CHECK(status IN ('queued','running','cancelling','completed','completed_with_errors','cancelled','error'))
);

CREATE INDEX IF NOT EXISTS idx_game_scan_jobs_status
    ON game_scan_jobs(status, id);
CREATE INDEX IF NOT EXISTS idx_game_scan_jobs_library
    ON game_scan_jobs(library_id, status, id DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase20 games catalog: %w", err)
	}
	return migratePhase21(db)
}
