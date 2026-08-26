package media

import (
	"context"
	"encoding/json"
	"strings"
)

type profileEpisodeState struct {
	Position  float64
	Completed bool
}

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
	states := map[int64]profileEpisodeState{}
	rows, err := s.db.QueryContext(ctx, `SELECT media_id,position_seconds,completed FROM profile_progress WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var state profileEpisodeState
		if err := rows.Scan(&id, &state.Position, &state.Completed); err != nil {
			_ = rows.Close()
			return nil, err
		}
		states[id] = state
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	groups, err := s.seriesGroups(ctx, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, item := range existing {
		seen[item.ID] = true
	}
	out := []Item{}
	for _, group := range groups {
		episodes := []Item{}
		for _, season := range group.Seasons {
			episodes = append(episodes, season.Episodes...)
		}
		lastCompleted := -1
		hasActiveEpisode := false
		for i, episode := range episodes {
			state, ok := states[episode.ID]
			if !ok {
				continue
			}
			if state.Completed {
				lastCompleted = i
			} else if state.Position >= 30 {
				hasActiveEpisode = true
			}
		}
		if hasActiveEpisode || lastCompleted < 0 || lastCompleted+1 >= len(episodes) {
			continue
		}
		next := episodes[lastCompleted+1]
		if seen[next.ID] {
			continue
		}
		if state, ok := states[next.ID]; ok && (state.Completed || state.Position >= 30) {
			continue
		}
		next.PositionSeconds = 0
		next.DurationSeconds = 0
		next.ProgressPercent = 0
		out = append(out, next)
		seen[next.ID] = true
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
