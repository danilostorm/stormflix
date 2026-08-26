package media

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type HistoryItem struct {
	Item
	Completed bool   `json:"completed"`
	WatchedAt string `json:"watched_at"`
}

type ProfileStats struct {
	WatchSeconds      float64 `json:"watch_seconds"`
	MonthWatchSeconds float64 `json:"month_watch_seconds"`
	CompletedTitles   int     `json:"completed_titles"`
	HistoryTitles     int     `json:"history_titles"`
	ActiveDays        int     `json:"active_days"`
	CurrentStreak     int     `json:"current_streak"`
}

type RankingEntry struct {
	Rank             int     `json:"rank"`
	ProfileID        int64   `json:"profile_id"`
	Name             string  `json:"name"`
	AvatarKey        string  `json:"avatar_key"`
	AvatarURL        string  `json:"avatar_url"`
	WatchSeconds     float64 `json:"watch_seconds"`
	CompletedTitles  int     `json:"completed_titles"`
	ActiveDays       int     `json:"active_days"`
}

func (s *Service) ProfileHistory(ctx context.Context, profileID int64, allowedLibraryIDs []int64, limit int) ([]HistoryItem, error) {
	if profileID <= 0 {
		return []HistoryItem{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []HistoryItem{}, nil
	}
	args := []any{profileID}
	query := `SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
pp.position_seconds,pp.duration_seconds,pp.completed,pp.updated_at
FROM profile_progress pp
JOIN media m ON m.id=pp.media_id
JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE pp.profile_id=? AND m.available=1 AND (pp.position_seconds>=30 OR pp.completed=1)`
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
	out := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		var genres string
		if err := rows.Scan(&h.ID, &h.LibraryID, &h.LibraryName, &h.Title, &h.Extension, &h.SizeBytes, &h.ModifiedUnix, &h.Available,
			&h.MediaType, &h.Year, &h.SeasonNumber, &h.EpisodeNumber, &h.Overview, &genres, &h.Rating, &h.RuntimeMinutes, &h.MetadataStatus, &h.TMDBID,
			&h.PosterURL, &h.BackdropURL, &h.LogoURL, &h.PositionSeconds, &h.DurationSeconds, &h.Completed, &h.WatchedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &h.Genres)
		if h.DurationSeconds > 0 {
			h.ProgressPercent = h.PositionSeconds / h.DurationSeconds * 100
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Physical copies must not duplicate one title in history.
	seen := map[string]bool{}
	clean := make([]HistoryItem, 0, len(out))
	for _, h := range out {
		key := strings.ToLower(strings.TrimSpace(h.Title)) + ":" + strings.TrimSpace(h.MediaType)
		if h.TMDBID > 0 {
			key = h.MediaType + ":tmdb:" + strconvFormatInt(h.TMDBID)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, h)
		if len(clean) >= limit {
			break
		}
	}
	return clean, nil
}

func (s *Service) ProfileStats(ctx context.Context, profileID int64) (ProfileStats, error) {
	var stats ProfileStats
	if profileID <= 0 {
		return stats, nil
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(seconds_watched),0),COUNT(DISTINCT watch_date) FROM profile_watch_daily WHERE profile_id=?`, profileID).Scan(&stats.WatchSeconds, &stats.ActiveDays)
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(seconds_watched),0) FROM profile_watch_daily WHERE profile_id=? AND watch_date>=date('now','start of month')`, profileID).Scan(&stats.MonthWatchSeconds)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_progress WHERE profile_id=? AND completed=1`, profileID).Scan(&stats.CompletedTitles)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_progress WHERE profile_id=? AND (position_seconds>=30 OR completed=1)`, profileID).Scan(&stats.HistoryTitles)

	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT watch_date FROM profile_watch_daily WHERE profile_id=? ORDER BY watch_date DESC LIMIT 120`, profileID)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	days := map[string]bool{}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return stats, err
		}
		days[day] = true
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	today := time.Now().UTC()
	start := today
	if !days[start.Format("2006-01-02")] {
		start = start.AddDate(0, 0, -1)
	}
	for i := 0; i < 120; i++ {
		if !days[start.Format("2006-01-02")] {
			break
		}
		stats.CurrentStreak++
		start = start.AddDate(0, 0, -1)
	}
	return stats, nil
}

func (s *Service) League(ctx context.Context, limit int) ([]RankingEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.name,p.avatar_key,p.avatar_url,
COALESCE(SUM(w.seconds_watched),0) watched,
COUNT(DISTINCT CASE WHEN w.completed=1 THEN w.media_id END) completed,
COUNT(DISTINCT w.watch_date) active_days
FROM profiles p
JOIN profile_watch_daily w ON w.profile_id=p.id AND w.watch_date>=date('now','start of month')
WHERE p.active=1
GROUP BY p.id,p.name,p.avatar_key,p.avatar_url
HAVING SUM(w.seconds_watched)>=60
ORDER BY watched DESC,completed DESC,active_days DESC,p.name COLLATE NOCASE
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RankingEntry{}
	for rows.Next() {
		var item RankingEntry
		if err := rows.Scan(&item.ProfileID, &item.Name, &item.AvatarKey, &item.AvatarURL, &item.WatchSeconds, &item.CompletedTitles, &item.ActiveDays); err != nil {
			return nil, err
		}
		item.Rank = len(out) + 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func strconvFormatInt(v int64) string {
	// Avoid pulling formatting concerns into callers; IDs are always positive.
	if v == 0 {
		return "0"
	}
	const digits = "0123456789"
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%10]
		v /= 10
	}
	return string(buf[i:])
}
