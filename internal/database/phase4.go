package database

import (
	"database/sql"
	"fmt"
)

func migratePhase4(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS library_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id INTEGER NOT NULL,
    path TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_library_sources_library ON library_sources(library_id, sort_order, id);

INSERT OR IGNORE INTO library_sources(library_id,path,label,enabled,sort_order)
SELECT id,path,'Origem 1',1,0 FROM libraries WHERE TRIM(path)<>'';

CREATE TRIGGER IF NOT EXISTS trg_library_default_source
AFTER INSERT ON libraries
WHEN TRIM(NEW.path)<>''
BEGIN
    INSERT OR IGNORE INTO library_sources(library_id,path,label,enabled,sort_order)
    VALUES(NEW.id,NEW.path,'Origem 1',1,0);
END;
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase4 database: %w", err)
	}
	return nil
}
