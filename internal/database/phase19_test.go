package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesPhase19CreditAnalysisState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	for _, name := range []string{
		"media_marker_segments",
		"idx_media_marker_segments_media",
		"media_credit_analysis",
		"idx_media_credit_analysis_status",
		"credit_analysis_jobs",
		"idx_credit_analysis_jobs_status",
		"idx_credit_analysis_jobs_season",
	} {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
			t.Fatalf("phase19 object %s missing: %v", name, err)
		}
	}
}

func TestPhase19AllowsSeparatedCreditSegments(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	library, err := db.Exec(`INSERT INTO libraries(name,kind,path) VALUES('TV','series','/tv')`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := library.LastInsertId()
	media, err := db.Exec(`INSERT INTO media(library_id,path,title,extension,available) VALUES(?, '/tv/episode.mkv', 'Episode', '.mkv', 1)`, libraryID)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	mediaID, _ := media.LastInsertId()

	for i, interval := range [][2]float64{{2400, 2460}, {2520, 2600}} {
		if _, err := db.Exec(`INSERT INTO media_marker_segments(media_id,kind,segment_index,start_seconds,end_seconds,source,confidence) VALUES(?,'credits',?,?,?,'automatic',0.9)`, mediaID, i, interval[0], interval[1]); err != nil {
			t.Fatalf("insert separated segment %d: %v", i, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_marker_segments WHERE media_id=? AND kind='credits'`, mediaID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("credit segment count=%d err=%v", count, err)
	}
}
