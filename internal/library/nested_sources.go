package library

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
)

// delegatedSourceRoots returns enabled source roots owned by other libraries
// that live strictly below root. Those more-specific roots reserve their
// subtree, so a broad parent library can coexist with dedicated child
// libraries without scanning the same files twice.
//
// A disabled library still keeps ownership of its configured enabled sources:
// disabling a library must hide that collection, not silently make its media
// appear in a broader parent library. Individual source enabled state remains
// authoritative when that control is used.
func (s *Service) delegatedSourceRoots(ctx context.Context, libraryID int64, root string) ([]string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return nil, err
	}
	rootAbs = filepath.Clean(rootAbs)

	// Some focused unit tests and pre-multi-source legacy databases contain the
	// original libraries/media schema without library_sources. Keep the legacy
	// scanner functional there; production migrations create this table before
	// multi-source ownership is used.
	var hasSourcesTable int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='library_sources'`).Scan(&hasSourcesTable); err != nil {
		return nil, err
	}
	if hasSourcesTable == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT path FROM library_sources
WHERE library_id<>? AND enabled=1
UNION
SELECT l.path FROM libraries l
WHERE l.id<>?
  AND NOT EXISTS (SELECT 1 FROM library_sources own WHERE own.library_id=l.id)`, libraryID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	roots := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if abs == rootAbs || !sameOrInside(abs, rootAbs) || seen[abs] {
			continue
		}
		seen[abs] = true
		roots = append(roots, abs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(roots)
	return roots, nil
}
