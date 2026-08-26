package database

import (
	"database/sql"
	"fmt"
)

// Phase 5 keeps the single-server SQLite design fast as catalogs grow. These
// indexes target the hottest StormFlix access patterns: library scans, home
// ordering, logical-provider lookups and per-profile Continue Watching.
func migratePhase5(db *sql.DB) error {
	const schema = `
CREATE INDEX IF NOT EXISTS idx_media_library_available_modified
    ON media(library_id, available, modified_unix DESC);
CREATE INDEX IF NOT EXISTS idx_media_library_path_available
    ON media(library_id, path, available);
CREATE INDEX IF NOT EXISTS idx_metadata_tmdb
    ON media_metadata(tmdb_id) WHERE tmdb_id > 0;
CREATE INDEX IF NOT EXISTS idx_metadata_type_year
    ON media_metadata(media_type, year);
CREATE INDEX IF NOT EXISTS idx_profile_progress_active
    ON profile_progress(profile_id, completed, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_library_sources_enabled
    ON library_sources(library_id, enabled, sort_order, id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase5 database: %w", err)
	}
	return nil
}
