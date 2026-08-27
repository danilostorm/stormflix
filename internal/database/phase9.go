package database

import (
	"database/sql"
	"fmt"
)

// migratePhase9 separates episodic scanning from metadata lookup. The scanner
// records the show identity derived from the configured library source and the
// path hierarchy before any external provider is queried.
func migratePhase9(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS media_series_identity (
    media_id INTEGER PRIMARY KEY,
    library_id INTEGER NOT NULL,
    source_root TEXT NOT NULL DEFAULT '',
    series_key TEXT NOT NULL DEFAULT '',
    series_title TEXT NOT NULL DEFAULT '',
    season_number INTEGER NOT NULL DEFAULT 0,
    episode_number INTEGER NOT NULL DEFAULT 0,
    absolute_number INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_media_series_identity_library ON media_series_identity(library_id);
CREATE INDEX IF NOT EXISTS idx_media_series_identity_key ON media_series_identity(library_id,series_key,season_number,episode_number);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase9 series identity: %w", err)
	}
	return nil
}
