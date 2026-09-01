package games

import (
	"context"
	"strings"
)

// AdminCatalogExact avoids multiplicative joins between ROM files, saves and
// profile state. Each aggregate is calculated independently so identical play
// times in two profiles are never collapsed by SUM(DISTINCT ...).
func (s *Service) AdminCatalogExact(ctx context.Context, query, platform string, limit int) ([]AdminGame, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	where := ` WHERE 1=1`
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		where += ` AND lower(g.title) LIKE ?`
		args = append(args, "%"+strings.ToLower(q)+"%")
	}
	if p := strings.ToLower(strings.TrimSpace(platform)); p != "" {
		where += ` AND g.platform=?`
		args = append(args, p)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT g.id,g.library_id,l.name,g.platform,g.title,g.content_hash,
       (SELECT COUNT(*) FROM game_files gf WHERE gf.game_id=g.id),
       (SELECT COUNT(*) FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1),
       (SELECT COUNT(*) FROM game_saves gs WHERE gs.game_id=g.id),
       COALESCE((SELECT SUM(gps.play_seconds) FROM game_profile_state gps WHERE gps.game_id=g.id),0),
       COALESCE(NULLIF(g.metadata_provider,''),gm.primary_provider,''),
       COALESCE(NULLIF(g.metadata_id,''),gm.primary_id,''),g.release_year,
       COALESCE(gm.metadata_locked,0),g.updated_at
FROM games g
JOIN libraries l ON l.id=g.library_id
LEFT JOIN game_metadata gm ON gm.game_id=g.id`+where+`
ORDER BY g.sort_title,g.title,g.id
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminGame{}
	for rows.Next() {
		var item AdminGame
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.Library, &item.Platform, &item.Title, &item.ContentHash, &item.FileCount, &item.AvailableFiles, &item.SaveCount, &item.PlaySeconds, &item.Provider, &item.MetadataID, &item.ReleaseYear, &item.MetadataLocked, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
