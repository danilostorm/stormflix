package admin

import "context"

func (s *Service) SaveProfileProgress(ctx context.Context, profileID, mediaID int64, positionSeconds, durationSeconds float64) error {
	if profileID <= 0 || mediaID <= 0 {
		return nil
	}
	completed := false
	if durationSeconds > 0 && positionSeconds/durationSeconds >= 0.92 {
		completed = true
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO profile_progress(profile_id,media_id,position_seconds,duration_seconds,completed,updated_at)
VALUES(?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,media_id) DO UPDATE SET position_seconds=excluded.position_seconds,duration_seconds=excluded.duration_seconds,completed=excluded.completed,updated_at=CURRENT_TIMESTAMP`,
		profileID, mediaID, positionSeconds, durationSeconds, completed)
	return err
}
