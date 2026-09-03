package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAppliesSQLitePragmasToEveryConnection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "storm flix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	connections := make([]*sql.Conn, 0, 4)
	for i := 0; i < 4; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire connection %d: %v", i, err)
		}
		connections = append(connections, conn)
	}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()

	for i, conn := range connections {
		assertPragmaInt(t, conn, i, "foreign_keys", 1)
		assertPragmaInt(t, conn, i, "busy_timeout", 5000)
		assertPragmaInt(t, conn, i, "synchronous", 1)
		assertPragmaInt(t, conn, i, "wal_autocheckpoint", 1000)
		var mode string
		if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatalf("connection %d journal_mode: %v", i, err)
		}
		if mode != "wal" {
			t.Fatalf("connection %d journal_mode=%q, want wal", i, mode)
		}
	}
}

func assertPragmaInt(t *testing.T, conn *sql.Conn, connection int, name string, want int) {
	t.Helper()
	var got int
	if err := conn.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&got); err != nil {
		t.Fatalf("connection %d %s: %v", connection, name, err)
	}
	if got != want {
		t.Fatalf("connection %d %s=%d, want %d", connection, name, got, want)
	}
}

func TestOpenRecordsSchemaMigrationBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stormflix.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var count, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != currentSchemaVersion {
		t.Fatalf("migration rows=%d, want %d", count, currentSchemaVersion)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("user_version=%d, want %d", version, currentSchemaVersion)
	}
}

func TestInspectReportsSQLiteHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stormflix.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	diagnostics := Inspect(context.Background(), db, path)
	if diagnostics.JournalMode != "wal" || !diagnostics.ForeignKeys || diagnostics.BusyTimeoutMS != 5000 {
		t.Fatalf("unexpected SQLite diagnostics: %+v", diagnostics)
	}
	if diagnostics.UserVersion != currentSchemaVersion || diagnostics.MigrationCount != currentSchemaVersion || diagnostics.PageCount == 0 || diagnostics.DatabaseBytes == 0 {
		t.Fatalf("incomplete SQLite diagnostics: %+v", diagnostics)
	}
}

func TestCatalogProjectionIndexesAreInstalled(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{
		"idx_catalog_entities_library_recent", "idx_catalog_entities_rating",
		"idx_catalog_entities_series_recent", "idx_catalog_entities_release",
		"idx_catalog_entities_title", "idx_catalog_entities_library_title",
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("projection index %s count=%d", name, count)
		}
	}
}
