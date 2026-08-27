package metadata

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// PrepareAnimationLibraryRepair clears only automatically generated metadata
// from a western-cartoon library before a full refresh. Physical media is never
// renamed, moved or deleted. Manual matches stay protected.
func (s *Service) PrepareAnimationLibraryRepair(ctx context.Context, libraryID int64) (int, error) {
	var kind string
	if err := s.db.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, libraryID).Scan(&kind); err != nil {
		return 0, err
	}
	if !strings.EqualFold(strings.TrimSpace(kind), "animation_series") {
		return 0, nil
	}

	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.path,COALESCE(mm.manual_match,0)
FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.library_id=? AND m.available=1 ORDER BY m.id`, libraryID)
	if err != nil {
		return 0, err
	}
	type repairItem struct {
		id     int64
		path   string
		manual bool
	}
	items := []repairItem{}
	for rows.Next() {
		var item repairItem
		if err := rows.Scan(&item.id, &item.path, &item.manual); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if !item.manual {
			items = append(items, item)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, item := range items {
		rawTitle := animationRawTitle(item.path)
		if _, err := tx.ExecContext(ctx, `UPDATE media SET title=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, rawTitle, item.id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_artwork WHERE media_id=?`, item.id); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_metadata WHERE media_id=? AND COALESCE(manual_match,0)=0`, item.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	for _, item := range items {
		if err := s.assets.RemoveTree(fmt.Sprintf("artwork/%d", item.id)); err != nil {
			return 0, fmt.Errorf("clean artwork %d: %w", item.id, err)
		}
	}
	return len(items), nil
}

func animationRawTitle(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.NewReplacer(".", " ", "_", " ").Replace(name)
	name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
	if name == "" {
		return "Episódio"
	}
	return name
}
