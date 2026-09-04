package database

import (
	"database/sql"
	"fmt"
)

const phase25Version = 25

// Phase 25 adds a rebuildable read model for Home cards. Physical media rows
// remain authoritative; this projection only prevents every Home request from
// walking all episodes and artwork repeatedly.
func migratePhase25(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("migrate phase25 begin: %w", err)
	}
	defer tx.Rollback()

	const schema = `
CREATE TABLE IF NOT EXISTS catalog_projection_state (
    id INTEGER PRIMARY KEY CHECK(id=1),
    source_revision INTEGER NOT NULL DEFAULT 1,
    built_revision INTEGER NOT NULL DEFAULT 0,
    built_at TEXT,
    last_error TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO catalog_projection_state(id,source_revision,built_revision) VALUES(1,1,0);

CREATE TABLE IF NOT EXISTS catalog_entities (
    entity_key TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    representative_media_id INTEGER NOT NULL,
    library_id INTEGER NOT NULL,
    library_name TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    extension TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    modified_unix INTEGER NOT NULL DEFAULT 0,
    media_type TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    overview TEXT NOT NULL DEFAULT '',
    genres_json TEXT NOT NULL DEFAULT '[]',
    rating REAL NOT NULL DEFAULT 0,
    runtime_minutes INTEGER NOT NULL DEFAULT 0,
    metadata_status TEXT NOT NULL DEFAULT 'pending',
    tmdb_id INTEGER NOT NULL DEFAULT 0,
    collection_tmdb_id INTEGER NOT NULL DEFAULT 0,
    collection_name TEXT NOT NULL DEFAULT '',
    release_date TEXT NOT NULL DEFAULT '',
    poster_url TEXT NOT NULL DEFAULT '',
    backdrop_url TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    series_id TEXT NOT NULL DEFAULT '',
    season_count INTEGER NOT NULL DEFAULT 0,
    episode_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_library_recent
    ON catalog_entities(library_id,modified_unix DESC,entity_key);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_rating
    ON catalog_entities(rating DESC,modified_unix DESC,entity_key) WHERE rating>0;
CREATE INDEX IF NOT EXISTS idx_catalog_entities_series_recent
    ON catalog_entities(entity_type,modified_unix DESC,entity_key);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_release
    ON catalog_entities(release_date DESC,entity_key) WHERE release_date<>'';
CREATE INDEX IF NOT EXISTS idx_catalog_entities_title
    ON catalog_entities(title COLLATE NOCASE,entity_key);
CREATE INDEX IF NOT EXISTS idx_catalog_entities_library_title
    ON catalog_entities(library_id,title COLLATE NOCASE,entity_key);

CREATE TABLE IF NOT EXISTS catalog_entity_members (
    media_id INTEGER PRIMARY KEY,
    entity_key TEXT NOT NULL,
    library_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_catalog_entity_members_entity
    ON catalog_entity_members(entity_key,media_id);
CREATE INDEX IF NOT EXISTS idx_catalog_entity_members_library
    ON catalog_entity_members(library_id,entity_key,media_id);

DROP TRIGGER IF EXISTS trg_catalog_dirty_media_insert;
DROP TRIGGER IF EXISTS trg_catalog_dirty_media_update;
DROP TRIGGER IF EXISTS trg_catalog_dirty_media_delete;
DROP TRIGGER IF EXISTS trg_catalog_dirty_metadata_insert;
DROP TRIGGER IF EXISTS trg_catalog_dirty_metadata_update;
DROP TRIGGER IF EXISTS trg_catalog_dirty_metadata_delete;
DROP TRIGGER IF EXISTS trg_catalog_dirty_artwork_insert;
DROP TRIGGER IF EXISTS trg_catalog_dirty_artwork_update;
DROP TRIGGER IF EXISTS trg_catalog_dirty_artwork_delete;
DROP TRIGGER IF EXISTS trg_catalog_dirty_series_insert;
DROP TRIGGER IF EXISTS trg_catalog_dirty_series_update;
DROP TRIGGER IF EXISTS trg_catalog_dirty_series_delete;
DROP TRIGGER IF EXISTS trg_catalog_dirty_library_insert;
DROP TRIGGER IF EXISTS trg_catalog_dirty_library_update;
DROP TRIGGER IF EXISTS trg_catalog_dirty_library_delete;

CREATE TRIGGER trg_catalog_dirty_media_insert AFTER INSERT ON media BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_media_update AFTER UPDATE OF library_id,title,extension,size_bytes,modified_unix,available ON media BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_media_delete AFTER DELETE ON media BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_metadata_insert AFTER INSERT ON media_metadata BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_metadata_update AFTER UPDATE ON media_metadata BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_metadata_delete AFTER DELETE ON media_metadata BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_artwork_insert AFTER INSERT ON media_artwork BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_artwork_update AFTER UPDATE ON media_artwork BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_artwork_delete AFTER DELETE ON media_artwork BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_series_insert AFTER INSERT ON media_series_identity BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_series_update AFTER UPDATE ON media_series_identity BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_series_delete AFTER DELETE ON media_series_identity BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_library_insert AFTER INSERT ON libraries BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_library_update AFTER UPDATE OF name,kind,enabled ON libraries BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
CREATE TRIGGER trg_catalog_dirty_library_delete AFTER DELETE ON libraries BEGIN
  UPDATE catalog_projection_state SET source_revision=source_revision+1 WHERE id=1;
END;
`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase25 catalog projection: %w", err)
	}
	if err := recordMigration(tx, phase25Version, "catalog-home-projection", "phase25-v1"); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", phase25Version)); err != nil {
		return fmt.Errorf("set sqlite user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate phase25 commit: %w", err)
	}
	return migratePhase26(db)
}
