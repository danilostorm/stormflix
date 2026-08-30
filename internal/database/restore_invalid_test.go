package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPendingRestoreQuarantinesInvalidSnapshotAndKeepsCurrentDatabase(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stormflix.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open current database: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('restore-invalid-test','current') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path+".restore", []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyPendingRestore(path); err != nil {
		t.Fatalf("invalid restore should be quarantined, not take server down: %v", err)
	}
	if _, err := os.Stat(path + ".restore"); !os.IsNotExist(err) {
		t.Fatalf("invalid staged restore should be moved away, stat err=%v", err)
	}
	matches, err := filepath.Glob(path + ".restore.invalid-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one quarantined restore, matches=%v err=%v", matches, err)
	}

	current, err := Open(path)
	if err != nil {
		t.Fatalf("open preserved current database: %v", err)
	}
	defer current.Close()
	var value string
	if err := current.QueryRow(`SELECT value FROM settings WHERE key='restore-invalid-test'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "current" {
		t.Fatalf("current database changed after invalid restore: %q", value)
	}
}
