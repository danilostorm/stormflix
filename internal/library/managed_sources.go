package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// EnsureManagedSources appends deployment-managed source roots to a library
// without removing roots already configured by the administrator. It is
// intentionally idempotent so container restarts cannot duplicate sources.
//
// Offline/FUSE sources are allowed here. ScanMulti remains responsible for
// deciding whether a source can currently be read and preserves the previous
// catalog when a configured remote is temporarily unavailable.
func (s *Service) EnsureManagedSources(ctx context.Context, name, kind string, paths []string) (ManagedLibrary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Filmes"
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "movies"
	}
	managed, err := normalizeSourcePaths(paths)
	if err != nil {
		return ManagedLibrary{}, err
	}
	if len(managed) == 0 {
		return ManagedLibrary{}, nil
	}

	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM libraries WHERE name = ? COLLATE NOCASE ORDER BY id LIMIT 1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return s.CreateMulti(ctx, name, kind, managed, true)
	}
	if err != nil {
		return ManagedLibrary{}, err
	}

	lib, err := s.ManagedGet(ctx, id)
	if err != nil {
		return ManagedLibrary{}, err
	}
	merged := append([]string(nil), lib.Paths...)
	if len(merged) == 0 && strings.TrimSpace(lib.Path) != "" {
		merged = append(merged, lib.Path)
	}

	changed := false
	for _, candidate := range managed {
		candidate = filepath.Clean(candidate)
		covered := false
		for _, existing := range merged {
			existing = filepath.Clean(existing)
			if candidate == existing || sameOrInside(candidate, existing) {
				covered = true
				break
			}
			if sameOrInside(existing, candidate) {
				return ManagedLibrary{}, fmt.Errorf("managed source %s overlaps existing source %s in library %q", candidate, existing, lib.Name)
			}
		}
		if covered {
			continue
		}
		merged = append(merged, candidate)
		changed = true
	}
	if !changed {
		return lib, nil
	}

	// Preserve the library's existing kind and enabled state. Deployment-managed
	// paths are additive; they never silently change how an existing library is
	// classified or re-enable a library an administrator deliberately disabled.
	return s.AdminUpdateMultiPreservingCatalog(ctx, lib.ID, lib.Name, lib.Kind, merged, lib.Enabled)
}
