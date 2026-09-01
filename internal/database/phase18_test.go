package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase18MarkerAnalysisJobs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, name := range []string{"marker_analysis_jobs", "idx_marker_analysis_jobs_status", "idx_marker_analysis_jobs_season"} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase18 object %s missing: %v", name, err)
		}
	}
}
