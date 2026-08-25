package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StartLibraryErrorJob reprocesses only media currently marked as metadata
// errors. Successfully matched titles are left untouched, which makes parser
// upgrades cheap even on very large libraries.
func (s *Service) StartLibraryErrorJob(ctx context.Context, libraryID int64) (Job, error) {
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM libraries WHERE id=?`, libraryID).Scan(&name); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	if s.running[libraryID] {
		s.mu.Unlock()
		return Job{}, errors.New("a metadata scan is already running for this library")
	}
	s.running[libraryID] = true
	s.mu.Unlock()

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN media_metadata mm ON mm.media_id=m.id WHERE m.library_id=? AND m.available=1 AND mm.status='error'`, libraryID).Scan(&total); err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	if total == 0 {
		s.setRunning(libraryID, false)
		return Job{}, errors.New("this library has no metadata errors to retry")
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO metadata_jobs(library_id,status,total,message) VALUES(?, 'queued', ?, 'retry errors')`, libraryID, total)
	if err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	id, _ := res.LastInsertId()
	job, err := s.Job(ctx, id)
	if err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	go s.runLibraryErrorJob(id, libraryID)
	return job, nil
}

func (s *Service) runLibraryErrorJob(jobID, libraryID int64) {
	defer s.setRunning(libraryID, false)
	ctx := context.Background()
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status='running',started_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.title FROM media m JOIN media_metadata mm ON mm.media_id=m.id WHERE m.library_id=? AND m.available=1 AND mm.status='error' ORDER BY m.id`, libraryID)
	if err != nil {
		s.finishJob(ctx, jobID, "failed", err.Error())
		return
	}
	type retryItem struct {
		id    int64
		title string
	}
	items := []retryItem{}
	for rows.Next() {
		var item retryItem
		if err := rows.Scan(&item.id, &item.title); err != nil {
			_ = rows.Close()
			s.finishJob(ctx, jobID, "failed", err.Error())
			return
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		s.finishJob(ctx, jobID, "failed", err.Error())
		return
	}

	processed, matched, failed := 0, 0, 0
	for _, item := range items {
		if err := s.RefreshMediaSmart(ctx, item.id); err != nil {
			failed++
		} else {
			matched++
		}
		processed++
		s.updateProgress(ctx, jobID, processed, matched, failed, item.title)
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			s.finishJob(ctx, jobID, "failed", ctx.Err().Error())
			return
		}
	}
	status := "completed"
	if failed > 0 {
		status = "completed_with_errors"
	}
	s.finishJob(ctx, jobID, status, fmt.Sprintf("error retry: %d recovered, %d still unmatched", matched, failed))
}
