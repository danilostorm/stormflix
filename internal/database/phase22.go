package database

import (
	"database/sql"
	"fmt"
)

// Phase 22 adds the Games administration/metadata foundation. Provider secrets
// are encrypted by the Games service before they reach SQLite; this table only
// stores the resulting ciphertext plus non-secret configuration. Rich metadata
// remains separated from the ROM identity owned by games/content_hash.
func migratePhase22(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS game_metadata (
    game_id INTEGER PRIMARY KEY,
    primary_provider TEXT NOT NULL DEFAULT '',
    primary_id TEXT NOT NULL DEFAULT '',
    igdb_id TEXT NOT NULL DEFAULT '',
    steamgriddb_id TEXT NOT NULL DEFAULT '',
    mobygames_id TEXT NOT NULL DEFAULT '',
    screenscraper_id TEXT NOT NULL DEFAULT '',
    retroachievements_id TEXT NOT NULL DEFAULT '',
    launchbox_id TEXT NOT NULL DEFAULT '',
    hasheous_id TEXT NOT NULL DEFAULT '',
    thegamesdb_id TEXT NOT NULL DEFAULT '',
    flashpoint_id TEXT NOT NULL DEFAULT '',
    hltb_id TEXT NOT NULL DEFAULT '',
    demozoo_id TEXT NOT NULL DEFAULT '',
    pouet_id TEXT NOT NULL DEFAULT '',
    csdb_id TEXT NOT NULL DEFAULT '',
    libretro_id TEXT NOT NULL DEFAULT '',
    genres_json TEXT NOT NULL DEFAULT '[]',
    developers_json TEXT NOT NULL DEFAULT '[]',
    publishers_json TEXT NOT NULL DEFAULT '[]',
    screenshots_json TEXT NOT NULL DEFAULT '[]',
    hero_path TEXT NOT NULL DEFAULT '',
    logo_path TEXT NOT NULL DEFAULT '',
    age_rating TEXT NOT NULL DEFAULT '',
    community_rating REAL NOT NULL DEFAULT 0,
    player_count INTEGER NOT NULL DEFAULT 0,
    metadata_locked INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    refreshed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(game_id) REFERENCES games(id) ON DELETE CASCADE,
    CHECK(metadata_locked IN (0,1)),
    CHECK(community_rating >= 0),
    CHECK(player_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_game_metadata_provider
    ON game_metadata(primary_provider, primary_id, game_id);
CREATE INDEX IF NOT EXISTS idx_game_metadata_locked
    ON game_metadata(metadata_locked, updated_at DESC, game_id);

CREATE TABLE IF NOT EXISTS game_provider_settings (
    provider TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 0,
    public_json TEXT NOT NULL DEFAULT '{}',
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK(enabled IN (0,1))
);

CREATE TABLE IF NOT EXISTS game_metadata_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER,
    status TEXT NOT NULL DEFAULT 'queued',
    progress INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    matched INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    provider TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE SET NULL,
    CHECK(status IN ('queued','running','completed','completed_with_errors','cancelled','error'))
);

CREATE INDEX IF NOT EXISTS idx_game_metadata_jobs_status
    ON game_metadata_jobs(status, id);
CREATE INDEX IF NOT EXISTS idx_game_metadata_jobs_library
    ON game_metadata_jobs(library_id, id DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase22 games admin/metadata: %w", err)
	}
	return migratePhase23(db)
}
