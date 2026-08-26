package admin

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SaveProfileProgress keeps compatibility with web/legacy callers. Ordered
// clients should use SaveProfileProgressOrdered so late asynchronous requests
// cannot overwrite a newer seek position.
func (s *Service) SaveProfileProgress(ctx context.Context, profileID, mediaID int64, positionSeconds, durationSeconds float64) error {
	_, err := s.SaveProfileProgressOrdered(ctx, profileID, mediaID, positionSeconds, durationSeconds, "legacy", time.Now().UnixNano(), time.Now().UnixMilli())
	return err
}

// SaveProfileProgressOrdered persists progress using event time + playback
// session + sequence as ordering metadata. Position is deliberately NOT used as
// an ordering rule because seeking backwards is a valid user action.
func (s *Service) SaveProfileProgressOrdered(ctx context.Context, profileID, mediaID int64, positionSeconds, durationSeconds float64, sessionID string, sequence, eventMS int64) (bool, error) {
	if profileID <= 0 || mediaID <= 0 {
		return false, nil
	}
	if positionSeconds < 0 {
		positionSeconds = 0
	}
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = "legacy"
	}
	if eventMS <= 0 {
		eventMS = time.Now().UnixMilli()
	}
	if sequence <= 0 {
		sequence = eventMS
	}

	previous := 0.0
	previousDuration := 0.0
	var oldCompleted bool
	var oldSession string
	var oldSequence, oldEventMS int64
	err := s.db.QueryRowContext(ctx, `SELECT position_seconds,duration_seconds,completed,COALESCE(progress_session,''),COALESCE(progress_sequence,0),COALESCE(progress_event_ms,0)
FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, mediaID).Scan(&previous, &previousDuration, &oldCompleted, &oldSession, &oldSequence, &oldEventMS)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	effectiveDuration := durationSeconds
	if effectiveDuration <= 0 {
		effectiveDuration = previousDuration
	}
	completed := effectiveDuration > 0 && positionSeconds/effectiveDuration >= 0.92

	result, err := s.db.ExecContext(ctx, `INSERT INTO profile_progress(
 profile_id,media_id,position_seconds,duration_seconds,completed,progress_session,progress_sequence,progress_event_ms,updated_at)
VALUES(?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,media_id) DO UPDATE SET
 position_seconds=excluded.position_seconds,
 duration_seconds=CASE WHEN excluded.duration_seconds>0 THEN excluded.duration_seconds ELSE profile_progress.duration_seconds END,
 completed=excluded.completed,
 progress_session=excluded.progress_session,
 progress_sequence=excluded.progress_sequence,
 progress_event_ms=excluded.progress_event_ms,
 updated_at=CURRENT_TIMESTAMP
WHERE profile_progress.progress_event_ms=0
   OR excluded.progress_event_ms>profile_progress.progress_event_ms
   OR (excluded.progress_event_ms=profile_progress.progress_event_ms
       AND excluded.progress_session=profile_progress.progress_session
       AND excluded.progress_sequence>profile_progress.progress_sequence)`,
		profileID, mediaID, positionSeconds, durationSeconds, completed, sessionID, sequence, eventMS)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, nil
	}

	// Calculate only credible forward playback for analytics. A large jump or a
	// backwards seek changes resume position but must not create fake watch time.
	delta := 0.0
	if err == nil || oldEventMS > 0 || oldSession != "" || oldSequence > 0 {
		if positionSeconds >= previous {
			delta = positionSeconds - previous
			if delta > 300 {
				delta = 0
			}
		}
	} else if positionSeconds > 0 && positionSeconds <= 180 {
		delta = positionSeconds
	}
	if delta < 0 {
		delta = 0
	}

	if delta > 0 || (completed && !oldCompleted) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO profile_watch_daily(profile_id,media_id,watch_date,seconds_watched,completed,updated_at)
VALUES(?,?,date('now'),?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,media_id,watch_date) DO UPDATE SET
 seconds_watched=profile_watch_daily.seconds_watched+excluded.seconds_watched,
 completed=MAX(profile_watch_daily.completed,excluded.completed),
 updated_at=CURRENT_TIMESTAMP`, profileID, mediaID, delta, completed)
	}
	return true, err
}
