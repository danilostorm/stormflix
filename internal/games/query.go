package games

import (
	"context"
	"database/sql"
	"fmt"
)

// Find resolves one game directly so large ROM libraries never depend on the
// browse page size for detail, cover authorization or favorite mutations.
func (s *Service) Find(ctx context.Context, id, profileID int64, allowed []int64) (Game, error) {
	where := ` WHERE g.id=? AND EXISTS(SELECT 1 FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1)`
	args := []any{profileID, id}
	clause, accessArgs := allowedClause("g.library_id", allowed)
	where += clause
	args = append(args, accessArgs...)

	var game Game
	var cover string
	err := s.db.QueryRowContext(ctx, `SELECT g.id,g.library_id,l.name,g.platform,g.title,g.overview,g.release_year,g.cover_path,COALESCE(ps.favorite,0),COALESCE(ps.play_seconds,0),COALESCE(ps.last_played_at,''),g.created_at FROM games g JOIN libraries l ON l.id=g.library_id LEFT JOIN game_profile_state ps ON ps.game_id=g.id AND ps.profile_id=?`+where, args...).
		Scan(&game.ID, &game.LibraryID, &game.Library, &game.Platform, &game.Title, &game.Overview, &game.ReleaseYear, &cover, &game.Favorite, &game.PlaySeconds, &game.LastPlayed, &game.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return Game{}, sql.ErrNoRows
		}
		return Game{}, err
	}
	if cover != "" {
		game.CoverURL = fmt.Sprintf("/api/v1/games/%d/cover", game.ID)
	}
	return game, nil
}
