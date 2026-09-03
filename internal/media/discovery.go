package media

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Trending returns top-level titles ranked from actual StormFlix playback.
// Viewer diversity has extra weight so a single profile looping one title does
// not dominate the rail.
func (s *Service) Trending(ctx context.Context, allowedLibraryIDs []int64, days, limit int) ([]Item, error) {
	if days <= 0 {
		days = 2
	}
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{fmt.Sprintf("-%d days", days)}
	query := `SELECT MIN(m.id) representative_id,
COUNT(DISTINCT pwd.profile_id) viewers,
SUM(pwd.seconds_watched) watched
FROM profile_watch_daily pwd
JOIN media m ON m.id=pwd.media_id AND m.available=1
LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE pwd.watch_date>=date('now', ?)`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		query += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	query += ` GROUP BY CASE
 WHEN COALESCE(mm.tmdb_id,0)>0 THEN COALESCE(mm.media_type,'')||':'||CAST(mm.tmdb_id AS TEXT)
 ELSE 'media:'||CAST(m.id AS TEXT) END
HAVING SUM(pwd.seconds_watched)>=60
ORDER BY (COUNT(DISTINCT pwd.profile_id)*900)+SUM(pwd.seconds_watched) DESC
LIMIT ?`
	args = append(args, limit*2)
	ids, err := scanDiscoveryIDs(ctx, s.db, query, args...)
	if err != nil {
		return nil, err
	}
	return s.topLevelItemsFromIDs(ctx, ids, allowedLibraryIDs, limit)
}

// Releases is based on provider release dates, not the day a file was scanned.
func (s *Service) Releases(ctx context.Context, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	return s.projectedReleases(ctx, allowedLibraryIDs, limit)
}

func scanDiscoveryIDs(ctx context.Context, db *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		var viewers int
		var watched float64
		if err := rows.Scan(&id, &viewers, &watched); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) topLevelItemsFromIDs(ctx context.Context, ids []int64, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	return s.projectedItemsFromMediaIDs(ctx, ids, allowedLibraryIDs, limit)
}
