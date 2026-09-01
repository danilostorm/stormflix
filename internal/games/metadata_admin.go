package games

import (
	"context"
	"database/sql"
)

func (s *Service) SetMetadataLock(ctx context.Context, gameID int64, locked bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM games WHERE id=?`, gameID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO game_metadata(game_id,metadata_locked)
VALUES(?,?)
ON CONFLICT(game_id) DO UPDATE SET metadata_locked=excluded.metadata_locked,updated_at=CURRENT_TIMESTAMP`, gameID, locked)
	return err
}
