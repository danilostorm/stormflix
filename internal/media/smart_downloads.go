package media

import (
	"context"
	"database/sql"
	"errors"
)

// SmartDownloadCandidates returns the next unwatched episodes for one logical
// series in scanner order. The selected profile is authoritative: completed
// episodes are skipped and partial progress is carried with the item so the
// Android offline player can resume without another catalog walk.
func (s *Service) SmartDownloadCandidates(ctx context.Context, profileID int64, seriesID string, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if profileID <= 0 {
		return nil, errors.New("profile is required")
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}

	detail, err := s.SeriesDetail(ctx, seriesID, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}

	type progress struct {
		position  float64
		duration  float64
		completed bool
	}
	progressByMedia := map[int64]progress{}
	rows, err := s.db.QueryContext(ctx, `
SELECT media_id, position_seconds, duration_seconds, completed
FROM profile_progress
WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var mediaID int64
		var p progress
		if err := rows.Scan(&mediaID, &p.position, &p.duration, &p.completed); err != nil {
			_ = rows.Close()
			return nil, err
		}
		progressByMedia[mediaID] = p
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := make([]Item, 0, limit)
	for _, season := range detail.Seasons {
		for _, episode := range season.Episodes {
			if !episode.Available {
				continue
			}
			p := progressByMedia[episode.ID]
			if p.completed || (p.duration > 0 && p.position/p.duration >= 0.92) {
				continue
			}
			episode.SeriesID = seriesID
			episode.PositionSeconds = p.position
			episode.DurationSeconds = p.duration
			if p.duration > 0 {
				episode.ProgressPercent = p.position / p.duration * 100
			}
			out = append(out, episode)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// ProfileProgress is intentionally tiny and used by Smart Downloads to refresh
// local/offline state without exposing another profile's history.
func (s *Service) ProfileProgress(ctx context.Context, profileID, mediaID int64) (position, duration float64, completed bool, err error) {
	err = s.db.QueryRowContext(ctx, `
SELECT position_seconds, duration_seconds, completed
FROM profile_progress
WHERE profile_id=? AND media_id=?`, profileID, mediaID).Scan(&position, &duration, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	return
}
