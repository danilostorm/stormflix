package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/library"
)

func TestNestedLibrarySourceUsesMostSpecificOwnership(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	movieRoot := filepath.Join(root, "Filmes")
	animeRoot := filepath.Join(movieRoot, "Animes Dublados")
	if err := os.MkdirAll(animeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	moviePath := filepath.Join(movieRoot, "Filme Principal.mp4")
	animePath := filepath.Join(animeRoot, "Anime S01E01.mp4")
	if err := os.WriteFile(moviePath, []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(animePath, []byte("anime"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := library.NewService(db)
	ctx := context.Background()
	parent, err := svc.CreateMulti(ctx, "Filmes", "movies", []string{movieRoot}, true)
	if err != nil {
		t.Fatalf("create parent library: %v", err)
	}
	// Before the child library exists, the broad parent legitimately sees both
	// files. This simulates a real catalog that is later split into a dedicated
	// sub-library.
	initial, err := svc.ScanMulti(ctx, parent.ID)
	if err != nil {
		t.Fatalf("initial parent scan: %v", err)
	}
	if initial.Files != 2 {
		t.Fatalf("initial parent scan found %d files, want 2", initial.Files)
	}

	child, err := svc.CreateMulti(ctx, "Animes Dublados", "anime_series", []string{animeRoot}, true)
	if err != nil {
		t.Fatalf("nested child source should be allowed: %v", err)
	}

	preview, err := svc.PreviewMulti(ctx, parent.ID)
	if err != nil {
		t.Fatalf("preview parent after delegation: %v", err)
	}
	if preview.Discovered != 1 {
		t.Fatalf("parent preview discovered %d files, want only the non-delegated movie", preview.Discovered)
	}

	childScan, err := svc.ScanMulti(ctx, child.ID)
	if err != nil {
		t.Fatalf("child scan: %v", err)
	}
	if childScan.Files != 1 {
		t.Fatalf("child scan found %d files, want 1", childScan.Files)
	}
	parentScan, err := svc.ScanMulti(ctx, parent.ID)
	if err != nil {
		t.Fatalf("parent rescan: %v", err)
	}
	if parentScan.Files != 1 {
		t.Fatalf("parent rescan found %d files, want 1", parentScan.Files)
	}

	var parentMovie, parentAnime, childAnime int
	if err := db.QueryRow(`SELECT available FROM media WHERE library_id=? AND path=?`, parent.ID, moviePath).Scan(&parentMovie); err != nil {
		t.Fatalf("read parent movie: %v", err)
	}
	if err := db.QueryRow(`SELECT available FROM media WHERE library_id=? AND path=?`, parent.ID, animePath).Scan(&parentAnime); err != nil {
		t.Fatalf("read delegated parent anime: %v", err)
	}
	if err := db.QueryRow(`SELECT available FROM media WHERE library_id=? AND path=?`, child.ID, animePath).Scan(&childAnime); err != nil {
		t.Fatalf("read child anime: %v", err)
	}
	if parentMovie != 1 || parentAnime != 0 || childAnime != 1 {
		t.Fatalf("unexpected ownership: parentMovie=%d parentAnime=%d childAnime=%d", parentMovie, parentAnime, childAnime)
	}
}

func TestNestedLibrarySourceStillRejectsExactDuplicateRoot(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mediaRoot := filepath.Join(root, "Media")
	if err := os.MkdirAll(mediaRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	svc := library.NewService(db)
	ctx := context.Background()
	if _, err := svc.CreateMulti(ctx, "Primeira", "movies", []string{mediaRoot}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateMulti(ctx, "Segunda", "movies", []string{mediaRoot}, true); err == nil {
		t.Fatal("expected exact duplicate root across libraries to be rejected")
	}
}

func TestSameLibraryStillRejectsParentAndChildSources(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	parent := filepath.Join(root, "Media")
	child := filepath.Join(parent, "Child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	svc := library.NewService(db)
	if _, err := svc.CreateMulti(context.Background(), "Redundante", "movies", []string{parent, child}, true); err == nil {
		t.Fatal("expected overlapping roots within one library to be rejected")
	}
}
