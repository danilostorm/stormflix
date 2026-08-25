package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestScanPreservesExistingAgentTitle(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE libraries (
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL,
 kind TEXT NOT NULL,
 path TEXT NOT NULL,
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE media (
 id INTEGER PRIMARY KEY,
 library_id INTEGER NOT NULL,
 title TEXT NOT NULL,
 path TEXT NOT NULL,
 extension TEXT NOT NULL,
 size_bytes INTEGER NOT NULL DEFAULT 0,
 modified_unix INTEGER NOT NULL DEFAULT 0,
 available INTEGER NOT NULL DEFAULT 1,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(library_id, path)
);`)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "Matrix (1999) (1080p BluRay x265 10bit).mkv")
	if err := os.WriteFile(mediaPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO libraries(id,name,kind,path) VALUES(1,'Filmes Antigos','movies',?)`, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(1,'Matrix',?,'.mkv',4,1,1)`, mediaPath); err != nil {
		t.Fatal(err)
	}

	s := &Service{db: db}
	if _, err := s.Scan(context.Background(), 1); err != nil {
		t.Fatal(err)
	}

	var title string
	if err := db.QueryRow(`SELECT title FROM media WHERE library_id=1 AND path=?`, mediaPath).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Matrix" {
		t.Fatalf("scan overwrote agent title with filename: %q", title)
	}
}
