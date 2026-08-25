package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Service) ValidateLibraryJob(ctx context.Context, libraryID int64) error {
	var enabled bool
	var scanStatus string
	var mediaCount int
	err := s.db.QueryRowContext(ctx, `SELECT l.enabled,COALESCE(l.last_scan_status,''),(SELECT COUNT(*) FROM media m WHERE m.library_id=l.id AND m.available=1) FROM libraries l WHERE l.id=?`, libraryID).Scan(&enabled, &scanStatus, &mediaCount)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("library is disabled")
	}
	if strings.EqualFold(scanStatus, "running") {
		return errors.New("library scan is still running; wait for it to finish before fetching metadata")
	}
	if mediaCount == 0 {
		return errors.New("library has no cataloged media; scan the library first")
	}
	return nil
}

func (s *Service) RecoverStaleJobs(maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}
	seconds := int64(maxAge.Seconds())
	res, err := s.db.Exec(`UPDATE metadata_jobs SET status='failed',message='job stalled and was recovered automatically',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running') AND (strftime('%s','now')-strftime('%s',updated_at))>?`, seconds)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Service) WatchJob(id int64, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 35 * time.Minute
	}
	timer := time.NewTimer(maxAge)
	defer timer.Stop()
	<-timer.C
	var status string
	if err := s.db.QueryRow(`SELECT status FROM metadata_jobs WHERE id=?`, id).Scan(&status); err != nil {
		return
	}
	if status != "queued" && status != "running" {
		return
	}
	message := fmt.Sprintf("metadata job exceeded %s and was stopped by watchdog", maxAge.Round(time.Minute))
	_, _ = s.db.Exec(`UPDATE metadata_jobs SET status='failed',message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('queued','running')`, message, id)
}
