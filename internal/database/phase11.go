package database

import (
	"database/sql"
	"fmt"
)

// Phase 11 finalizes the Plex-style principal-series matching model. Older
// builds marked every episode as manual when a series override was chosen;
// series-level protection now lives only in series_metadata_overrides.
func migratePhase11(db *sql.DB) error {
	const schema = `
CREATE INDEX IF NOT EXISTS idx_series_identity_order
    ON media_series_identity(library_id, series_key, season_number, episode_number, media_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase11 principal series matching: %w", err)
	}
	if _, err := db.Exec(`UPDATE media_metadata SET manual_match=0,updated_at=CURRENT_TIMESTAMP
WHERE media_id IN (
    SELECT si.media_id
    FROM media_series_identity si
    JOIN series_metadata_overrides smo
      ON smo.library_id=si.library_id AND smo.series_key=si.series_key
    WHERE smo.manual=1
)`); err != nil {
		return fmt.Errorf("normalize series-level manual flags: %w", err)
	}
	return nil
}
