package library

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
)

// AdminUpdateMultiPreservingCatalog updates a library and, when one or more
// source roots were replaced one-for-one, rewrites existing media paths to the
// new roots before the normal source cleanup runs. Media IDs therefore remain
// stable, so metadata, artwork, progress and other media_id-owned state do not
// need to be downloaded or rebuilt merely because a Drive/rclone mount moved.
func (s *Service) AdminUpdateMultiPreservingCatalog(ctx context.Context, id int64, name, kind string, paths []string, enabled bool) (ManagedLibrary, error) {
	name = strings.TrimSpace(name)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if name == "" {
		return ManagedLibrary{}, errors.New("library name is required")
	}
	cleanPaths, err := normalizeSourcePaths(paths)
	if err != nil {
		return ManagedLibrary{}, err
	}
	if len(cleanPaths) == 0 {
		return ManagedLibrary{}, errors.New("at least one library source is required")
	}
	if err := s.ValidateLibraryPaths(ctx, id, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}
	oldSources, err := s.Sources(ctx, id)
	if err != nil {
		return ManagedLibrary{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagedLibrary{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `UPDATE libraries SET name=?,kind=?,path=?,enabled=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, kind, cleanPaths[0], enabled, id)
	if err != nil {
		return ManagedLibrary{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ManagedLibrary{}, sql.ErrNoRows
	}

	if err := relocateChangedSourceRootsTx(ctx, tx, id, oldSources, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}
	if err := replaceSourcesTx(ctx, tx, id, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}
	if err := markRemovedSourcesUnavailable(ctx, tx, id, oldSources, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedLibrary{}, err
	}
	return s.ManagedGet(ctx, id)
}

// relocateChangedSourceRootsTx pairs only roots that truly disappeared with
// roots that are truly new. Pure source reordering therefore never rewrites
// media paths. Relocation is intentionally conservative: a one-for-one mapping
// is required so StormFlix never guesses when sources were added/removed in an
// ambiguous way.
func relocateChangedSourceRootsTx(ctx context.Context, tx *sql.Tx, libraryID int64, oldSources []LibrarySource, newPaths []string) error {
	oldSet := map[string]bool{}
	newSet := map[string]bool{}
	for _, source := range oldSources {
		oldSet[filepath.Clean(source.Path)] = true
	}
	for _, path := range newPaths {
		newSet[filepath.Clean(path)] = true
	}

	removed := make([]string, 0)
	for _, source := range oldSources {
		root := filepath.Clean(source.Path)
		if !newSet[root] {
			removed = append(removed, root)
		}
	}
	added := make([]string, 0)
	for _, path := range newPaths {
		root := filepath.Clean(path)
		if !oldSet[root] {
			added = append(added, root)
		}
	}
	if len(removed) == 0 || len(removed) != len(added) {
		return nil
	}
	for i := range removed {
		if err := relocateMediaRootTx(ctx, tx, libraryID, removed[i], added[i]); err != nil {
			return err
		}
	}
	return nil
}

func relocateMediaRootTx(ctx context.Context, tx *sql.Tx, libraryID int64, oldRoot, newRoot string) error {
	oldRoot = filepath.Clean(oldRoot)
	newRoot = filepath.Clean(newRoot)
	if oldRoot == newRoot {
		return nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT id,path FROM media WHERE library_id=?`, libraryID)
	if err != nil {
		return err
	}
	type mediaPath struct {
		id   int64
		path string
	}
	items := make([]mediaPath, 0)
	for rows.Next() {
		var item mediaPath
		if err := rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return err
		}
		if sameOrInside(filepath.Clean(item.path), oldRoot) {
			items = append(items, item)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, item := range items {
		rel, err := filepath.Rel(oldRoot, filepath.Clean(item.path))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Clean(filepath.Join(newRoot, rel))

		var collisionID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM media WHERE library_id=? AND path=? AND id<>? LIMIT 1`, libraryID, target, item.id).Scan(&collisionID)
		if err == nil {
			// Never destroy or merge two catalog identities implicitly. A later
			// scan can resolve a real duplicate using the normal consolidation
			// workflow.
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}

		if _, err := tx.ExecContext(ctx, `UPDATE media SET path=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, target, item.id); err != nil {
			return err
		}
		// Series identity is keyed by media_id, so only its physical source root
		// needs to move. Scanner series/season/episode identity remains intact.
		if _, err := tx.ExecContext(ctx, `UPDATE media_series_identity SET source_root=?,updated_at=CURRENT_TIMESTAMP WHERE media_id=? AND source_root=?`, newRoot, item.id, oldRoot); err != nil {
			return err
		}
	}
	return nil
}
