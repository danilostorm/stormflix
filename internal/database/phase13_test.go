package database

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase13AutomationSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stormflix.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"media_technical", "catalog_changes", "profile_home_menus", "system_backups"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("phase13 table %s missing: %v", table, err)
		}
	}
	for _, column := range []string{"playback_session_id", "buffer_seconds", "read_mbps", "cache_bytes"} {
		if !testColumnExists(t, db, "playback_sessions", column) {
			t.Fatalf("phase13 playback_sessions column %s missing", column)
		}
	}
}

func TestApplyPendingRestoreValidatesAndActivatesSQLiteSnapshot(t *testing.T) {
	root := t.TempDir()
	currentPath := filepath.Join(root, "stormflix.db")
	current, err := Open(currentPath)
	if err != nil {
		t.Fatalf("open current database: %v", err)
	}
	if _, err := current.Exec(`INSERT INTO settings(key,value) VALUES('restore-test','current') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		t.Fatal(err)
	}
	if _, err := current.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	sourcePath := filepath.Join(root, "source.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatalf("open restore source: %v", err)
	}
	if _, err := source.Exec(`INSERT INTO settings(key,value) VALUES('restore-test','restored') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	if err := copyTestFile(sourcePath, currentPath+".restore"); err != nil {
		t.Fatalf("stage restore: %v", err)
	}
	if err := applyPendingRestore(currentPath); err != nil {
		t.Fatalf("apply pending restore: %v", err)
	}
	if _, err := os.Stat(currentPath + ".restore"); !os.IsNotExist(err) {
		t.Fatalf("staged restore should be consumed, stat err=%v", err)
	}

	restored, err := Open(currentPath)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow(`SELECT value FROM settings WHERE key='restore-test'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "restored" {
		t.Fatalf("expected restored database marker, got %q", value)
	}
	matches, err := filepath.Glob(currentPath + ".pre-restore-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one pre-restore safety copy, matches=%v err=%v", matches, err)
	}
}

func testColumnExists(t *testing.T, db interface{ Query(string, ...any) (*sql.Rows, error) }, table, wanted string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == wanted {
			return true
		}
	}
	return false
}

func copyTestFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
