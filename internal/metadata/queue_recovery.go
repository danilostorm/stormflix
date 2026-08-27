package metadata

import "context"

// ResumeQueuedJobs restores only the queue types that have a deterministic
// resumable runner. Full-library metadata jobs from an older process are marked
// failed so they can be explicitly restarted; principal-series refreshes resume
// automatically because their provider choice and series key are persisted.
func (s *Service) ResumeQueuedJobs() {
	ctx := context.Background()
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status='failed',message='servidor reiniciado; reinicie este job pelo painel',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running') AND job_type<>'series_refresh'`)
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status='queued',started_at=NULL,finished_at=NULL,message='retomando atualização da obra principal após reinício',updated_at=CURRENT_TIMESTAMP WHERE status='running' AND job_type='series_refresh'`)
	rows, err := s.db.QueryContext(ctx, `SELECT id,library_id,series_key,series_title,provider_id FROM metadata_jobs WHERE status='queued' AND job_type='series_refresh' ORDER BY id`)
	if err != nil {
		return
	}
	type pending struct {
		id, libraryID, providerID int64
		seriesKey, seriesTitle    string
	}
	items := []pending{}
	for rows.Next() {
		var item pending
		if rows.Scan(&item.id, &item.libraryID, &item.seriesKey, &item.seriesTitle, &item.providerID) == nil {
			items = append(items, item)
		}
	}
	_ = rows.Close()
	for _, item := range items {
		go s.runQueuedManualSeries(item.id, item.libraryID, item.seriesKey, item.seriesTitle, item.providerID)
	}
}
