package database

import (
	"database/sql"
	"fmt"
)

// Phase 13 adds the persisted state used by catalog automation. The physical
// media rows remain the source of truth; these tables only cache technical
// inspection, presentation rules, profile menu choices, audit history and
// backup metadata.
func migratePhase13(db *sql.DB) error {
	for _, column := range []struct {
		table, name, definition string
	}{
		{"library_categories", "rule_mode", "TEXT NOT NULL DEFAULT 'libraries'"},
		{"library_categories", "rules_json", "TEXT NOT NULL DEFAULT '{}'"},
		{"playback_sessions", "playback_session_id", "TEXT NOT NULL DEFAULT ''"},
		{"playback_sessions", "mode", "TEXT NOT NULL DEFAULT 'direct_play'"},
		{"playback_sessions", "client_kind", "TEXT NOT NULL DEFAULT ''"},
		{"playback_sessions", "bitrate_kbps", "INTEGER NOT NULL DEFAULT 0"},
		{"playback_sessions", "buffer_seconds", "REAL NOT NULL DEFAULT 0"},
		{"playback_sessions", "read_mbps", "REAL NOT NULL DEFAULT 0"},
		{"playback_sessions", "cache_bytes", "INTEGER NOT NULL DEFAULT 0"},
		{"playback_sessions", "video_codec", "TEXT NOT NULL DEFAULT ''"},
		{"playback_sessions", "audio_codec", "TEXT NOT NULL DEFAULT ''"},
		{"playback_sessions", "last_error", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}

	const schema = `
CREATE TABLE IF NOT EXISTS media_technical (
    media_id INTEGER PRIMARY KEY,
    source_modified_unix INTEGER NOT NULL DEFAULT 0,
    source_json TEXT NOT NULL DEFAULT '{}',
    video_codec TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    hdr TEXT NOT NULL DEFAULT '',
    bitrate_kbps INTEGER NOT NULL DEFAULT 0,
    duration_seconds REAL NOT NULL DEFAULT 0,
    audio_json TEXT NOT NULL DEFAULT '[]',
    subtitle_json TEXT NOT NULL DEFAULT '[]',
    audio_pt_br INTEGER NOT NULL DEFAULT 0,
    subtitle_pt_br INTEGER NOT NULL DEFAULT 0,
    dub_status TEXT NOT NULL DEFAULT 'desconhecido',
    status TEXT NOT NULL DEFAULT 'pending',
    last_error TEXT NOT NULL DEFAULT '',
    probed_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_media_technical_dub ON media_technical(dub_status,media_id);
CREATE INDEX IF NOT EXISTS idx_media_technical_resolution ON media_technical(width,height,hdr,media_id);

CREATE TABLE IF NOT EXISTS catalog_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    before_json TEXT NOT NULL DEFAULT '',
    after_json TEXT NOT NULL DEFAULT '',
    user_id INTEGER,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_catalog_changes_created ON catalog_changes(id DESC);
CREATE INDEX IF NOT EXISTS idx_catalog_changes_entity ON catalog_changes(entity_type,entity_id,id DESC);

CREATE TABLE IF NOT EXISTS profile_home_menus (
    profile_id INTEGER NOT NULL,
    category_id INTEGER NOT NULL,
    visible INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(profile_id,category_id),
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(category_id) REFERENCES library_categories(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_profile_home_menus_order ON profile_home_menus(profile_id,visible,sort_order,category_id);

CREATE TABLE IF NOT EXISTS system_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL DEFAULT 'manual',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'ready',
    note TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_system_backups_created ON system_backups(id DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase13 catalog automation: %w", err)
	}
	return migratePhase14(db)
}
