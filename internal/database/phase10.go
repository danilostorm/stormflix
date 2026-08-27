package database

import (
	"database/sql"
	"fmt"
)

// Phase 10 adds three pieces that become important as a StormFlix catalog grows:
// persistent series-level manual matches, hierarchical categories, and indexes
// for the read-heavy Home/catalog paths. SQLite remains the supported database.
func migratePhase10(db *sql.DB) error {
	if err := ensureColumn(db, "library_categories", "parent_id", "INTEGER REFERENCES library_categories(id) ON DELETE SET NULL"); err != nil {
		return err
	}
	const schema = `
CREATE TABLE IF NOT EXISTS series_metadata_overrides (
    library_id INTEGER NOT NULL,
    series_key TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'tmdb',
    provider_id INTEGER NOT NULL,
    media_type TEXT NOT NULL DEFAULT 'tv',
    title TEXT NOT NULL DEFAULT '',
    manual INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(library_id, series_key),
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_series_override_provider
    ON series_metadata_overrides(provider, provider_id);
CREATE INDEX IF NOT EXISTS idx_series_identity_series
    ON media_series_identity(library_id, series_key, media_id);
CREATE INDEX IF NOT EXISTS idx_categories_parent_sort
    ON library_categories(parent_id, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_artwork_selected_lookup
    ON media_artwork(media_id, kind, selected, score DESC);
CREATE INDEX IF NOT EXISTS idx_media_available_modified_global
    ON media(available, modified_unix DESC, id);
CREATE INDEX IF NOT EXISTS idx_media_available_title
    ON media(available, title COLLATE NOCASE, id);
CREATE INDEX IF NOT EXISTS idx_metadata_rating_media
    ON media_metadata(rating DESC, media_id);
CREATE INDEX IF NOT EXISTS idx_metadata_status_media
    ON media_metadata(status, media_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase10 hierarchy and performance: %w", err)
	}
	return migratePhase11(db)
}
