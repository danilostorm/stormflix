package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestEnsureManagedSourcesAppendsWithoutRemovingExisting(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewService(db)
	ctx := context.Background()
	original := filepath.Join(t.TempDir(), "original")
	alien := filepath.Join(t.TempDir(), "alien")
	local := filepath.Join(t.TempDir(), "local")

	created, err := s.CreateMulti(ctx, "Filmes", "movies", []string{original}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureManagedSources(ctx, "Filmes", "movies", []string{alien, local}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureManagedSources(ctx, "Filmes", "movies", []string{alien, local}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ManagedGet(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Clean(original), filepath.Clean(alien), filepath.Clean(local)}
	if len(got.Paths) != len(want) {
		t.Fatalf("got %d paths (%v), want %d (%v)", len(got.Paths), got.Paths, len(want), want)
	}
	for i := range want {
		if filepath.Clean(got.Paths[i]) != want[i] {
			t.Fatalf("path[%d]=%q, want %q", i, got.Paths[i], want[i])
		}
	}
}

func TestEnsureManagedSourcesCreatesLibraryWhenMissing(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewService(db)
	managed := []string{filepath.Join(t.TempDir(), "one"), filepath.Join(t.TempDir(), "two")}
	got, err := s.EnsureManagedSources(context.Background(), "Filmes", "movies", managed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Filmes" || got.Kind != "movies" || len(got.Paths) != 2 {
		t.Fatalf("unexpected managed library: %+v", got)
	}
}
