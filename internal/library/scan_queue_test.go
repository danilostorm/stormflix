package library_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/library"
)

func TestEnqueueAllAdminScansCompletesPersistentQueue(t *testing.T) {
	root := t.TempDir()
	db, err := database.Open(filepath.Join(root, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	first := filepath.Join(root, "Filmes")
	second := filepath.Join(root, "Series")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "Filme Teste.mp4"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "Serie S01E01.mkv"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := library.NewService(db)
	if _, err := svc.CreateMulti(context.Background(), "Filmes", "movies", []string{first}, true); err != nil {
		t.Fatalf("create movies library: %v", err)
	}
	if _, err := svc.CreateMulti(context.Background(), "Séries", "series", []string{second}, true); err != nil {
		t.Fatalf("create series library: %v", err)
	}

	jobs, err := svc.EnqueueAllAdminScans(context.Background())
	if err != nil {
		t.Fatalf("enqueue all scans: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 queued scan jobs, got %d", len(jobs))
	}
	if jobs[0].ID >= jobs[1].ID {
		t.Fatalf("expected FIFO job ids, got %d then %d", jobs[0].ID, jobs[1].ID)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		listed, err := svc.ScanJobs(context.Background(), 10)
		if err != nil {
			t.Fatalf("list scan jobs: %v", err)
		}
		completed := 0
		for _, job := range listed {
			if job.Status == "completed" || job.Status == "completed_with_errors" {
				completed++
			}
			if job.Status == "error" || job.Status == "timeout" {
				t.Fatalf("scan job %d failed: %s", job.ID, job.Message)
			}
		}
		if completed == 2 {
			var available int
			if err := db.QueryRow(`SELECT COUNT(*) FROM media WHERE available=1`).Scan(&available); err != nil {
				t.Fatalf("count media: %v", err)
			}
			if available != 2 {
				t.Fatalf("expected 2 scanned media rows, got %d", available)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("scan queue did not finish within timeout")
}
