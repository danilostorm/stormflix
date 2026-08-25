package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (s *Service) ValidateLibraryPath(ctx context.Context, ignoreID int64, candidate string) error {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,path FROM libraries WHERE id<>?`, ignoreID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, path string
		if err := rows.Scan(&id, &name, &path); err != nil {
			return err
		}
		existing, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		existing = filepath.Clean(existing)
		if sameOrInside(abs, existing) || sameOrInside(existing, abs) {
			return fmt.Errorf("library path overlaps %q (%s); choose the specific media folder instead of a parent/child library root", name, existing)
		}
	}
	return rows.Err()
}

func sameOrInside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
