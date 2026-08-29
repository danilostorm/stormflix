package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/library"
)

func TestLibraryRootRelocationPreservesMediaIdentityAndMetadata(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	oldRoot := filepath.Join(root, "drive-antigo", "Filmes")
	newRoot := filepath.Join(root, "drive-novo", "Filmes")
	relative := filepath.Join("Acao", "Filme Teste (2026).mp4")
	for _, base := range []string{oldRoot, newRoot} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(base, relative)), 0o755); err != nil {
			t.Fatalf("mkdir media dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(base, relative), []byte("same-media"), 0o644); err != nil {
			t.Fatalf("write media: %v", err)
		}
	}

	svc := library.NewService(db)
	created, err := svc.CreateMulti(context.Background(), "Filmes", "movies", []string{oldRoot}, true)
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := svc.ScanMulti(context.Background(), created.ID); err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	var mediaID int64
	if err := db.QueryRow(`SELECT id FROM media WHERE library_id=? AND path=?`, created.ID, filepath.Join(oldRoot, relative)).Scan(&mediaID); err != nil {
		t.Fatalf("find original media: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO media_metadata(media_id,status,provider,provider_id) VALUES(?, 'matched', 'tmdb', '12345')`, mediaID); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO media_artwork(media_id,kind,provider,source_url,public_url,selected) VALUES(?, 'poster', 'tmdb', 'https://example.invalid/poster.jpg', '/assets/poster.jpg', 1)`, mediaID); err != nil {
		t.Fatalf("seed artwork: %v", err)
	}

	if _, err := svc.AdminUpdateMultiPreservingCatalog(context.Background(), created.ID, "Filmes", "movies", []string{newRoot}, true); err != nil {
		t.Fatalf("move library root: %v", err)
	}

	var movedID int64
	var movedPath string
	if err := db.QueryRow(`SELECT id,path FROM media WHERE library_id=?`, created.ID).Scan(&movedID, &movedPath); err != nil {
		t.Fatalf("read moved media: %v", err)
	}
	if movedID != mediaID {
		t.Fatalf("media identity changed after root relocation: before=%d after=%d", mediaID, movedID)
	}
	wantPath := filepath.Join(newRoot, relative)
	if filepath.Clean(movedPath) != filepath.Clean(wantPath) {
		t.Fatalf("unexpected moved path: got %q want %q", movedPath, wantPath)
	}

	var provider, providerID, status string
	if err := db.QueryRow(`SELECT provider,provider_id,status FROM media_metadata WHERE media_id=?`, mediaID).Scan(&provider, &providerID, &status); err != nil {
		t.Fatalf("metadata was lost: %v", err)
	}
	if provider != "tmdb" || providerID != "12345" || status != "matched" {
		t.Fatalf("metadata changed unexpectedly: provider=%q id=%q status=%q", provider, providerID, status)
	}
	var artwork int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_artwork WHERE media_id=? AND selected=1`, mediaID).Scan(&artwork); err != nil {
		t.Fatalf("count artwork: %v", err)
	}
	if artwork != 1 {
		t.Fatalf("selected artwork was not preserved: %d", artwork)
	}

	// A subsequent scan must update the existing row instead of creating a new
	// media_id, which is what keeps metadata/artwork from being downloaded again.
	if _, err := svc.ScanMulti(context.Background(), created.ID); err != nil {
		t.Fatalf("scan after relocation: %v", err)
	}
	var total int
	var afterScanID int64
	if err := db.QueryRow(`SELECT COUNT(*),MIN(id) FROM media WHERE library_id=? AND available=1`, created.ID).Scan(&total, &afterScanID); err != nil {
		t.Fatalf("count media after relocation scan: %v", err)
	}
	if total != 1 || afterScanID != mediaID {
		t.Fatalf("scan recreated media after relocation: count=%d id=%d want id=%d", total, afterScanID, mediaID)
	}
}
