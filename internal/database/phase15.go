package database

import (
	"database/sql"
	"fmt"
)

// Phase 15 adds per-profile external-account state and query indexes used by
// the large-catalog Home path. Trakt credentials are encrypted by the settings
// secret codec before they are persisted in these tables.
func migratePhase15(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS profile_trakt (
    profile_id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    token_expires_at TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    user_slug TEXT NOT NULL DEFAULT '',
    connected_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS profile_trakt_device_auth (
    profile_id INTEGER PRIMARY KEY,
    device_code TEXT NOT NULL,
    user_code TEXT NOT NULL,
    verification_url TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL DEFAULT 5,
    requested_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_media_artwork_selected_lookup
    ON media_artwork(media_id, kind, selected, score DESC);
CREATE INDEX IF NOT EXISTS idx_media_available_library_title
    ON media(available, library_id, title COLLATE NOCASE, id);
CREATE INDEX IF NOT EXISTS idx_media_modified_available
    ON media(available, modified_unix DESC, id);
CREATE INDEX IF NOT EXISTS idx_metadata_movie_collection_backfill
    ON media_metadata(media_type, tmdb_id, collection_source_tmdb_id, collection_checked_at, media_id);
CREATE INDEX IF NOT EXISTS idx_metadata_collection_browse
    ON media_metadata(media_type, collection_tmdb_id, collection_name, year, media_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase15 profile integrations and home indexes: %w", err)
	}
	return nil
}
