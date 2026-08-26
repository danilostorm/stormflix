package admin

import (
	"context"
	"database/sql"
)

func (s *Service) SaveProfileProgress(ctx context.Context, profileID, mediaID int64, positionSeconds, durationSeconds float64) error {
	if profileID <= 0 || mediaID <= 0 {
		return nil
	}
	completed := durationSeconds > 0 && positionSeconds/durationSeconds >= 0.92

	// Calculate only credible forward playback. Large jumps are seeks/resumes and
	// must not turn into artificial ranking minutes or trending popularity.
	previous := 0.0
	var oldCompleted bool
	err := s.db.QueryRowContext(ctx, `SELECT position_seconds,completed FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, mediaID).Scan(&previous, &oldCompleted)
	delta := 0.0
	if err == nil {
		if positionSeconds >= previous {
			delta = positionSeconds - previous
			if delta > 300 {
				delta = 0
			}
		}
	} else if err == sql.ErrNoRows && positionSeconds > 0 && positionSeconds <= 180 {
		delta = positionSeconds
	} else if err != nil && err != sql.ErrNoRows {
		return err
	}
	if delta < 0 {
		delta = 0
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO profile_progress(profile_id,media_id,position_seconds,duration_seconds,completed,updated_at)
VALUES(?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,media_id) DO UPDATE SET position_seconds=excluded.position_seconds,duration_seconds=excluded.duration_seconds,completed=excluded.completed,updated_at=CURRENT_TIMESTAMP`,
		profileID, mediaID, positionSeconds, durationSeconds, completed)
	if err != nil {
		return err
	}

	if delta > 0 || (completed && !oldCompleted) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO profile_watch_daily(profile_id,media_id,watch_date,seconds_watched,completed,updated_at)
VALUES(?,?,date('now'),?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,media_id,watch_date) DO UPDATE SET
 seconds_watched=profile_watch_daily.seconds_watched+excluded.seconds_watched,
 completed=MAX(profile_watch_daily.completed,excluded.completed),
 updated_at=CURRENT_TIMESTAMP`, profileID, mediaID, delta, completed)
	}
	return err
}
