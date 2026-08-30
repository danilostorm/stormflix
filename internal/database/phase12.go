package database

import (
	"database/sql"
	"fmt"
)

// Phase 12 adds persistent operational queues. Scan work is intentionally
// serialized so large rclone/FUSE libraries do not compete for IO, while the
// same Admin queue view can also expose metadata/series refresh activity.
func migratePhase12(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS scan_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    progress INTEGER NOT NULL DEFAULT 0,
    files INTEGER NOT NULL DEFAULT 0,
    sources_total INTEGER NOT NULL DEFAULT 0,
    sources_scanned INTEGER NOT NULL DEFAULT 0,
    sources_offline INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_status_id ON scan_jobs(status,id);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_library_status ON scan_jobs(library_id,status,id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase12 operational queues: %w", err)
	}
	for _, column := range []struct {
		table, name, definition string
	}{
		{"metadata_jobs", "job_type", "TEXT NOT NULL DEFAULT 'library_metadata'"},
		{"metadata_jobs", "series_key", "TEXT NOT NULL DEFAULT ''"},
		{"metadata_jobs", "series_title", "TEXT NOT NULL DEFAULT ''"},
		{"metadata_jobs", "provider_id", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := ensureColumn(db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	// A server restart should not lose queued work. A job that was actually
	// running is safe to place back at the head of the queue because scans and
	// metadata writes are idempotent/upsert based.
	if _, err := db.Exec(`UPDATE scan_jobs SET status='queued',progress=0,started_at=NULL,finished_at=NULL,message='retomado após reinício do servidor',updated_at=CURRENT_TIMESTAMP WHERE status IN ('running','cancelling')`); err != nil {
		return fmt.Errorf("recover scan queue: %w", err)
	}
	if _, err := db.Exec(`UPDATE metadata_jobs SET status='queued',started_at=NULL,finished_at=NULL,message=CASE WHEN job_type='series_refresh' THEN 'retomando atualização da obra principal após reinício' ELSE 'retomando job após reinício' END,updated_at=CURRENT_TIMESTAMP WHERE status='running'`); err != nil {
		return fmt.Errorf("recover metadata queue: %w", err)
	}
	return migratePhase13(db)
}
