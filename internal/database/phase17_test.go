package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase17AutomaticIntroAnalysis(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var table string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='media_marker_analysis'`).Scan(&table); err != nil {
		t.Fatalf("phase17 analysis table missing: %v", err)
	}
	var index string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_media_marker_analysis_status'`).Scan(&index); err != nil {
		t.Fatalf("phase17 analysis index missing: %v", err)
	}
}
