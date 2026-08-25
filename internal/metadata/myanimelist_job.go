package metadata

import (
	"context"
	"time"
)

// RetryLibraryErrorsWhenJobDone watches the real job status and starts the MAL
// second pass immediately after the normal metadata scan completes.
func (s *Service) RetryLibraryErrorsWhenJobDone(jobID, libraryID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var status string
			if err := s.db.QueryRowContext(ctx, `SELECT status FROM metadata_jobs WHERE id=?`, jobID).Scan(&status); err != nil {
				return
			}
			switch status {
			case "completed", "completed_with_errors":
				s.RetryLibraryErrorsWithMyAnimeList(ctx, libraryID)
				return
			case "failed":
				return
			}
		}
	}
}
