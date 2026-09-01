package database

import (
	"database/sql"
	"fmt"
)

// Phase 21 adds profile-owned game saves and durable play-session accounting.
// Save payloads remain on disk under the StormFlix data directory; SQLite keeps
// only bounded metadata/version information so large emulator blobs never bloat
// the main catalog database.
func migratePhase21(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS game_saves (
    profile_id INTEGER NOT NULL,
    game_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    slot INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(profile_id, game_id, kind, slot),
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(game_id) REFERENCES games(id) ON DELETE CASCADE,
    CHECK(kind IN ('state','sram')),
    CHECK(slot >= 0 AND slot <= 9),
    CHECK(size_bytes >= 0),
    CHECK(version >= 1)
);

CREATE INDEX IF NOT EXISTS idx_game_saves_profile_updated
    ON game_saves(profile_id, updated_at DESC, game_id, kind, slot);
CREATE INDEX IF NOT EXISTS idx_game_saves_game
    ON game_saves(game_id, profile_id, kind, slot);

CREATE TABLE IF NOT EXISTS game_play_sessions (
    session_id TEXT PRIMARY KEY,
    profile_id INTEGER NOT NULL,
    game_id INTEGER NOT NULL,
    client_seconds INTEGER NOT NULL DEFAULT 0,
    credited_seconds INTEGER NOT NULL DEFAULT 0,
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(game_id) REFERENCES games(id) ON DELETE CASCADE,
    CHECK(client_seconds >= 0),
    CHECK(credited_seconds >= 0)
);

CREATE INDEX IF NOT EXISTS idx_game_play_sessions_profile
    ON game_play_sessions(profile_id, last_seen_at DESC, game_id);
CREATE INDEX IF NOT EXISTS idx_game_play_sessions_seen
    ON game_play_sessions(last_seen_at, session_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase21 game saves/play sessions: %w", err)
	}
	return migratePhase22(db)
}
