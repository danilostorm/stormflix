package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestAutomaticBackupCreatesValidSnapshotAndReusesRecentCopy(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('backup-test','present')`); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db}
	first, err := s.ensureAutomaticBackup(context.Background(), "test safety backup")
	if err != nil {
		t.Fatalf("create automatic backup: %v", err)
	}
	firstID, ok := first["id"].(int64)
	if !ok || firstID <= 0 {
		t.Fatalf("unexpected backup id payload: %#v", first)
	}
	var path, status string
	if err := db.QueryRow(`SELECT path,status FROM system_backups WHERE id=?`, firstID).Scan(&path, &status); err != nil {
		t.Fatalf("read backup registry: %v", err)
	}
	if status != "ready" {
		t.Fatalf("backup status=%q", status)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("backup file missing/empty: info=%v err=%v", info, err)
	}

	check, err := database.Open(path)
	if err != nil {
		t.Fatalf("backup is not a valid StormFlix SQLite snapshot: %v", err)
	}
	var value string
	if err := check.QueryRow(`SELECT value FROM settings WHERE key='backup-test'`).Scan(&value); err != nil {
		_ = check.Close()
		t.Fatalf("read backed-up setting: %v", err)
	}
	_ = check.Close()
	if value != "present" {
		t.Fatalf("backup setting=%q want present", value)
	}

	second, err := s.ensureAutomaticBackup(context.Background(), "second operation")
	if err != nil {
		t.Fatalf("reuse recent automatic backup: %v", err)
	}
	if reused, _ := second["reused"].(bool); !reused {
		t.Fatalf("expected recent safety backup reuse, got %#v", second)
	}
	if secondID, _ := second["id"].(int64); secondID != firstID {
		t.Fatalf("reused backup id=%d want %d", secondID, firstID)
	}
}
