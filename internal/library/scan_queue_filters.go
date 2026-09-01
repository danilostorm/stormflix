package library

import "context"

// EnqueueAllAdminScansExceptGames keeps the existing video/music scanner away
// from native game libraries. Games have their own hash-based persistent queue.
func (s *Service) EnqueueAllAdminScansExceptGames(ctx context.Context) ([]ScanJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM libraries WHERE enabled=1 AND lower(trim(kind))<>'games' ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	jobs := make([]ScanJob, 0, len(ids))
	for _, id := range ids {
		_, job, err := s.EnqueueAdminScan(ctx, id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}
