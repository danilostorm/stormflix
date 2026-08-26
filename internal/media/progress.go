package media

import (
	"context"
	"encoding/json"
	"strings"
)

func (s *Service) ContinueWatching(ctx context.Context, profileID int64, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if profileID <= 0 {
		return []Item{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{profileID}
	query := `SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
pp.position_seconds,pp.duration_seconds
FROM profile_progress pp
JOIN media m ON m.id=pp.media_id
JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE pp.profile_id=? AND pp.completed=0 AND m.available=1
  AND pp.position_seconds>=30 AND pp.duration_seconds>=60
  AND (pp.position_seconds/pp.duration_seconds)<0.92`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		query += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	query += ` ORDER BY pp.updated_at DESC LIMIT ?`
	args = append(args, limit*3)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var item Item
		var genres string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available,
			&item.MediaType, &item.Year, &item.SeasonNumber, &item.EpisodeNumber, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID,
			&item.PosterURL, &item.BackdropURL, &item.LogoURL, &item.PositionSeconds, &item.DurationSeconds); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		if item.DurationSeconds > 0 {
			item.ProgressPercent = item.PositionSeconds / item.DurationSeconds * 100
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items = DedupeItems(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
