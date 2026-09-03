package database

import (
	"context"
	"database/sql"
	"os"
)

type Diagnostics struct {
	JournalMode      string `json:"journal_mode"`
	Synchronous      int    `json:"synchronous"`
	ForeignKeys      bool   `json:"foreign_keys"`
	BusyTimeoutMS    int    `json:"busy_timeout_ms"`
	UserVersion      int    `json:"user_version"`
	MigrationCount   int    `json:"migration_count"`
	PageSize         int64  `json:"page_size"`
	PageCount        int64  `json:"page_count"`
	FreePages        int64  `json:"free_pages"`
	DatabaseBytes    int64  `json:"database_bytes"`
	WALBytes         int64  `json:"wal_bytes"`
	OpenConnections  int    `json:"open_connections"`
	InUseConnections int    `json:"in_use_connections"`
	IdleConnections  int    `json:"idle_connections"`
	WaitCount        int64  `json:"wait_count"`
}

// Inspect returns a cheap read-only snapshot used by Admin diagnostics. It
// makes the SQLite-to-PostgreSQL decision measurable instead of speculative.
func Inspect(ctx context.Context, db *sql.DB, path string) Diagnostics {
	var out Diagnostics
	_ = db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&out.JournalMode)
	_ = db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&out.Synchronous)
	var foreignKeys int
	_ = db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys)
	out.ForeignKeys = foreignKeys == 1
	_ = db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&out.BusyTimeoutMS)
	_ = db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&out.UserVersion)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&out.MigrationCount)
	_ = db.QueryRowContext(ctx, `PRAGMA page_size`).Scan(&out.PageSize)
	_ = db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&out.PageCount)
	_ = db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&out.FreePages)
	if info, err := os.Stat(path); err == nil {
		out.DatabaseBytes = info.Size()
	}
	if info, err := os.Stat(path + "-wal"); err == nil {
		out.WALBytes = info.Size()
	}
	stats := db.Stats()
	out.OpenConnections = stats.OpenConnections
	out.InUseConnections = stats.InUse
	out.IdleConnections = stats.Idle
	out.WaitCount = stats.WaitCount
	return out
}
