package database

import (
	"database/sql"
	"fmt"
)

func migratePhase7(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS music_tracks (
    media_id INTEGER PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    artist TEXT NOT NULL DEFAULT '',
    album_artist TEXT NOT NULL DEFAULT '',
    album TEXT NOT NULL DEFAULT '',
    track_number INTEGER NOT NULL DEFAULT 0,
    disc_number INTEGER NOT NULL DEFAULT 0,
    year INTEGER NOT NULL DEFAULT 0,
    genre TEXT NOT NULL DEFAULT '',
    duration_seconds REAL NOT NULL DEFAULT 0,
    codec TEXT NOT NULL DEFAULT '',
    bitrate INTEGER NOT NULL DEFAULT 0,
    sample_rate INTEGER NOT NULL DEFAULT 0,
    channels INTEGER NOT NULL DEFAULT 0,
    musicbrainz_track_id TEXT NOT NULL DEFAULT '',
    musicbrainz_album_id TEXT NOT NULL DEFAULT '',
    musicbrainz_artist_id TEXT NOT NULL DEFAULT '',
    indexed_modified_unix INTEGER NOT NULL DEFAULT 0,
    indexed_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS music_albums (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    album_key TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    artist TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    musicbrainz_release_id TEXT NOT NULL DEFAULT '',
    cover_url TEXT NOT NULL DEFAULT '',
    last_enriched_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS music_listening_daily (
    day TEXT NOT NULL,
    profile_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    listened_seconds REAL NOT NULL DEFAULT 0,
    plays INTEGER NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0,
    last_played_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(day, profile_id, media_id),
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS music_favorites (
    profile_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(profile_id, media_id),
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS music_playlists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS music_playlist_items (
    playlist_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(playlist_id, media_id),
    FOREIGN KEY (playlist_id) REFERENCES music_playlists(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS music_lyrics (
    media_id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL DEFAULT '',
    provider_id TEXT NOT NULL DEFAULT '',
    plain_lyrics TEXT NOT NULL DEFAULT '',
    synced_lyrics TEXT NOT NULL DEFAULT '',
    instrumental INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_music_tracks_artist ON music_tracks(artist COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_music_tracks_album ON music_tracks(album COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_music_listening_recent ON music_listening_daily(last_played_at DESC);
CREATE INDEX IF NOT EXISTS idx_music_listening_media ON music_listening_daily(media_id, day);
CREATE INDEX IF NOT EXISTS idx_music_favorites_profile ON music_favorites(profile_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_music_playlists_profile ON music_playlists(profile_id, updated_at DESC);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase7 music: %w", err)
	}
	return nil
}
