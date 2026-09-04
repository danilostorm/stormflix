package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := applyPendingRestore(path); err != nil {
		return nil, fmt.Errorf("apply staged database restore: %w", err)
	}

	dsn, err := sqliteDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// StormFlix is a single-server workload. A small pool avoids excessive
	// competing SQLite writers while WAL still lets readers continue during
	// scans, progress heartbeats and metadata updates.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	// modernc applies _pragma parameters whenever database/sql creates a new
	// physical connection. This is required for foreign_keys, busy_timeout and
	// synchronous, which are connection-local SQLite settings. Executing these
	// once through db.Exec only configured whichever pooled connection happened
	// to service that call.
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase2(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase3(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase4(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase5(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase6(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase7(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase8(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase9(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migratePhase10(db); err != nil {
		db.Close()
		return nil, err
	}
	// Refresh planner statistics opportunistically after schema/index changes.
	_, _ = db.Exec("PRAGMA optimize;")
	return db, nil
}

func sqliteDSN(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sqlite path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	query := url.Values{}
	for _, pragma := range []string{
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"foreign_keys(ON)",
		"busy_timeout(5000)",
		"wal_autocheckpoint(1000)",
		// Home Query v2 uses bounded window queries. Keep their temporary
		// B-trees in memory and give each of the four pooled connections a
		// modest page cache instead of spilling hot catalog pages to disk.
		"temp_store(MEMORY)",
		"cache_size(-32768)",
		"mmap_size(268435456)",
	} {
		query.Add("_pragma", pragma)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS libraries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'movies',
    path TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    path TEXT NOT NULL,
    extension TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    modified_unix INTEGER NOT NULL DEFAULT 0,
    available INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE,
    UNIQUE(library_id, path)
);

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    user_agent TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_libraries (
    user_id INTEGER NOT NULL,
    library_id INTEGER NOT NULL,
    PRIMARY KEY(user_id, library_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS activity_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL DEFAULT 'info',
    category TEXT NOT NULL DEFAULT 'system',
    message TEXT NOT NULL,
    user_id INTEGER,
    details TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS playback_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    device TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE,
    UNIQUE(user_id, media_id, device)
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_media_library_id ON media(library_id);
CREATE INDEX IF NOT EXISTS idx_media_title ON media(title);
CREATE INDEX IF NOT EXISTS idx_media_available ON media(available);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_logs_created_at ON activity_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_playback_last_seen ON playback_sessions(last_seen_at);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	columns := []struct{ table, name, definition string }{
		{"libraries", "enabled", "INTEGER NOT NULL DEFAULT 1"},
		{"libraries", "last_scan_at", "TEXT"},
		{"libraries", "last_scan_status", "TEXT NOT NULL DEFAULT 'never'"},
		{"libraries", "last_error", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range columns {
		if err := ensureColumn(db, c.table, c.name, c.definition); err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(db *sql.DB, table, name, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if columnName == name {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, name, err)
	}
	return nil
}
