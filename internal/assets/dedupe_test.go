package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeduplicateKeepsPathsAndContent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(root, "artwork", "1", "poster.jpg"),
		filepath.Join(root, "artwork", "2", "poster.jpg"),
		filepath.Join(root, "artwork", "3", "poster.jpg"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte("same-poster-content")
	if err := os.WriteFile(paths[0], payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[1], payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths[2], []byte("different-poster"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := store.Deduplicate()
	if err != nil {
		t.Fatal(err)
	}
	if report.LinkedFiles != 1 || report.SavedBytes != int64(len(payload)) {
		t.Fatalf("unexpected report: %+v", report)
	}
	first, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(first, second) {
		t.Fatal("duplicate files were not consolidated to the same inode")
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("asset path disappeared after dedupe: %s: %v", path, err)
		}
	}
}

func TestDeduplicateIsIdempotent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("duplicate"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Deduplicate(); err != nil {
		t.Fatal(err)
	}
	report, err := store.Deduplicate()
	if err != nil {
		t.Fatal(err)
	}
	if report.LinkedFiles != 0 || report.SavedBytes != 0 {
		t.Fatalf("second pass should be a no-op, got %+v", report)
	}
}
