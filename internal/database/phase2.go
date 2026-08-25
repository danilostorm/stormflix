package database

import (
	"database/sql"
	"fmt"
)

func migratePhase2(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS media_metadata (
    media_id INTEGER PRIMARY KEY,
    media_type TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    season_number INTEGER NOT NULL DEFAULT 0,
    episode_number INTEGER NOT NULL DEFAULT 0,
    overview TEXT NOT NULL DEFAULT '',
    genres_json TEXT NOT NULL DEFAULT '[]',
    rating REAL NOT NULL DEFAULT 0,
    runtime_minutes INTEGER NOT NULL DEFAULT 0,
    provider TEXT NOT NULL DEFAULT '',
    provider_id TEXT NOT NULL DEFAULT '',
    tmdb_id INTEGER NOT NULL DEFAULT 0,
    tvdb_id INTEGER NOT NULL DEFAULT 0,
    imdb_id TEXT NOT NULL DEFAULT '',
    anilist_id INTEGER NOT NULL DEFAULT 0,
    mal_id INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_artwork (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    provider TEXT NOT NULL,
    source_url TEXT NOT NULL DEFAULT '',
    asset_path TEXT NOT NULL DEFAULT '',
    public_url TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0,
    selected INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE,
    UNIQUE(media_id, kind, provider, source_url)
);

CREATE TABLE IF NOT EXISTS subtitles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    language TEXT NOT NULL,
    hearing_impaired INTEGER NOT NULL DEFAULT 0,
    format TEXT NOT NULL DEFAULT 'srt',
    provider TEXT NOT NULL DEFAULT '',
    provider_id TEXT NOT NULL DEFAULT '',
    release_name TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    asset_path TEXT NOT NULL DEFAULT '',
    public_url TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE,
    UNIQUE(media_id, language, provider, provider_id)
);

CREATE TABLE IF NOT EXISTS metadata_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    matched INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS subtitle_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    language TEXT NOT NULL DEFAULT 'pt-BR',
    status TEXT NOT NULL DEFAULT 'queued',
    total INTEGER NOT NULL DEFAULT 0,
    processed INTEGER NOT NULL DEFAULT 0,
    downloaded INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_metadata_status ON media_metadata(status);
CREATE INDEX IF NOT EXISTS idx_artwork_media_kind ON media_artwork(media_id, kind, selected);
CREATE INDEX IF NOT EXISTS idx_subtitles_media_language ON subtitles(media_id, language);
CREATE INDEX IF NOT EXISTS idx_metadata_jobs_library ON metadata_jobs(library_id, created_at);
CREATE INDEX IF NOT EXISTS idx_subtitle_jobs_library ON subtitle_jobs(library_id, created_at);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase2 database: %w", err)
	}

	columns := []struct{ table, name, definition string }{
		{"media_metadata", "original_title", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "tagline", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "cast_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"media_metadata", "directors_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"media_metadata", "trailer_url", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "theme_preview_url", "TEXT NOT NULL DEFAULT ''"},
		{"media_metadata", "theme_preview_title", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range columns {
		if err := ensureColumn(db, c.table, c.name, c.definition); err != nil {
			return err
		}
	}
	return nil
}
