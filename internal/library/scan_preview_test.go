package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/library"
)

func TestPreviewMultiReportsChangesWithoutMutatingCatalog(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	mediaRoot := filepath.Join(root, "Filmes")
	if err := os.MkdirAll(mediaRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(mediaRoot, "Antigo.mp4")
	if err := os.WriteFile(oldPath, []byte("old-file"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := library.NewService(db)
	created, err := svc.CreateMulti(context.Background(), "Filmes", "movies", []string{mediaRoot}, true)
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := svc.ScanMulti(context.Background(), created.ID); err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(mediaRoot, "Novo.mp4")
	if err := os.WriteFile(newPath, []byte("new-file"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview, err := svc.PreviewMulti(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("preview scan: %v", err)
	}
	if preview.New != 1 || preview.Missing != 1 || preview.Discovered != 1 || preview.Existing != 1 {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	var oldAvailable int
	if err := db.QueryRow(`SELECT available FROM media WHERE library_id=? AND path=?`, created.ID, oldPath).Scan(&oldAvailable); err != nil {
		t.Fatalf("read old catalog row: %v", err)
	}
	if oldAvailable != 1 {
		t.Fatalf("preview changed old media availability to %d", oldAvailable)
	}
	var newCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media WHERE library_id=? AND path=?`, created.ID, newPath).Scan(&newCount); err != nil {
		t.Fatalf("count new catalog row: %v", err)
	}
	if newCount != 0 {
		t.Fatalf("preview inserted %d new catalog rows", newCount)
	}
}
