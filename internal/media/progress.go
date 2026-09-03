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
	items := []Item{}
	for rows.Next() {
		var item Item
		var genres string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available,
			&item.MediaType, &item.Year, &item.SeasonNumber, &item.EpisodeNumber, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID,
			&item.PosterURL, &item.BackdropURL, &item.LogoURL, &item.PositionSeconds, &item.DurationSeconds); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		if item.DurationSeconds > 0 {
			item.ProgressPercent = item.PositionSeconds / item.DurationSeconds * 100
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items = DedupeItems(items)

	// When an episode was completed, surface the next unplayed episode even
	// though its position is still zero. This keeps TV/mobile Continue Watching
	// moving naturally through a series instead of making it disappear.
	if len(items) < limit {
		next, nextErr := s.nextEpisodesAfterCompleted(ctx, profileID, allowedLibraryIDs, limit-len(items), items)
		if nextErr == nil {
			items = append(items, next...)
			items = DedupeItems(items)
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *Service) nextEpisodesAfterCompleted(ctx context.Context, profileID int64, allowedLibraryIDs []int64, limit int, existing []Item) ([]Item, error) {
	if limit <= 0 {
		return []Item{}, nil
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{}
	where := `m.available=1 AND l.enabled=1 AND si.series_key<>'' AND si.episode_number>0`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		where += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	query := `WITH ordered AS (
  SELECT si.media_id,si.library_id,si.series_key,
         ROW_NUMBER() OVER (
           PARTITION BY si.library_id,si.series_key
           ORDER BY si.season_number,si.episode_number,si.absolute_number,si.media_id
         ) AS sequence
  FROM media_series_identity si
  JOIN media m ON m.id=si.media_id
  JOIN libraries l ON l.id=m.library_id
  WHERE ` + where + `
), series_state AS (
  SELECT o.library_id,o.series_key,
         MAX(CASE WHEN pp.completed=1 THEN o.sequence ELSE 0 END) AS completed_sequence,
         MAX(CASE WHEN pp.completed=0 AND pp.position_seconds>=30 THEN 1 ELSE 0 END) AS has_active,
         MAX(CASE WHEN pp.completed=1 THEN pp.updated_at ELSE '' END) AS last_completed_at
  FROM ordered o
  LEFT JOIN profile_progress pp ON pp.media_id=o.media_id AND pp.profile_id=?
  GROUP BY o.library_id,o.series_key
), candidates AS (
  SELECT o.media_id,st.last_completed_at,
         ROW_NUMBER() OVER (PARTITION BY o.library_id,o.series_key ORDER BY o.sequence) AS candidate_rank
  FROM ordered o
  JOIN series_state st ON st.library_id=o.library_id AND st.series_key=o.series_key
  WHERE st.has_active=0 AND st.completed_sequence>0 AND o.sequence>st.completed_sequence
), next_ids AS (
  SELECT media_id,last_completed_at FROM candidates
  WHERE candidate_rank=1
  ORDER BY last_completed_at DESC
  LIMIT ?
), ranked_art AS (
  SELECT media_id,kind,public_url,
         ROW_NUMBER() OVER (PARTITION BY media_id,kind ORDER BY score DESC,id DESC) AS rank
  FROM media_artwork WHERE selected=1
), selected_art AS (
  SELECT media_id,
         MAX(CASE WHEN kind='poster' THEN public_url ELSE '' END) AS poster_url,
         MAX(CASE WHEN kind='backdrop' THEN public_url ELSE '' END) AS backdrop_url,
         MAX(CASE WHEN kind='logo' THEN public_url ELSE '' END) AS logo_url
  FROM ranked_art WHERE rank=1 GROUP BY media_id
)
SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(NULLIF(mm.media_type,''),'series'),COALESCE(mm.year,0),si.season_number,si.episode_number,
COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),
COALESCE(a.poster_url,''),COALESCE(a.backdrop_url,''),COALESCE(a.logo_url,'')
FROM next_ids n
JOIN media m ON m.id=n.media_id
JOIN libraries l ON l.id=m.library_id
JOIN media_series_identity si ON si.media_id=m.id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN selected_art a ON a.media_id=m.id
ORDER BY n.last_completed_at DESC,m.id`
	args = append(args, profileID, limit+len(existing)+8)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for _, item := range existing {
		seen[item.ID] = true
	}
	out := []Item{}
	for rows.Next() {
		var next Item
		var genres string
		if err := rows.Scan(&next.ID, &next.LibraryID, &next.LibraryName, &next.Title, &next.Extension, &next.SizeBytes, &next.ModifiedUnix, &next.Available,
			&next.MediaType, &next.Year, &next.SeasonNumber, &next.EpisodeNumber, &next.Overview, &genres, &next.Rating, &next.RuntimeMinutes, &next.MetadataStatus, &next.TMDBID,
			&next.PosterURL, &next.BackdropURL, &next.LogoURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &next.Genres)
		if seen[next.ID] {
			continue
		}
		out = append(out, next)
		seen[next.ID] = true
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
