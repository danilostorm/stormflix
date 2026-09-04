package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type LibrarySource struct {
	ID        int64  `json:"id"`
	LibraryID int64  `json:"library_id"`
	Path      string `json:"path"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
	Online    bool   `json:"online"`
}

type MultiScanResult struct {
	LibraryID      int64 `json:"library_id"`
	Files          int   `json:"files"`
	DurationMS     int64 `json:"duration_ms"`
	SourcesScanned int   `json:"sources_scanned"`
	SourcesOffline int   `json:"sources_offline"`
}

func (s *Service) Sources(ctx context.Context, libraryID int64) ([]LibrarySource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,library_id,path,label,enabled,sort_order FROM library_sources WHERE library_id=? ORDER BY sort_order,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibrarySource{}
	for rows.Next() {
		var source LibrarySource
		if err := rows.Scan(&source.ID, &source.LibraryID, &source.Path, &source.Label, &source.Enabled, &source.SortOrder); err != nil {
			return nil, err
		}
		// ONLINE is informational only. A FUSE/rclone stat may time out even
		// when directory listing itself still works, so scans never gate on it.
		source.Online = source.Enabled && dirOnline(source.Path)
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *Service) scanSources(ctx context.Context, libraryID int64) ([]LibrarySource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,library_id,path,label,enabled,sort_order FROM library_sources WHERE library_id=? ORDER BY sort_order,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibrarySource{}
	for rows.Next() {
		var source LibrarySource
		if err := rows.Scan(&source.ID, &source.LibraryID, &source.Path, &source.Label, &source.Enabled, &source.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *Service) ValidateLibraryPaths(ctx context.Context, ignoreID int64, candidates []string) error {
	paths, err := normalizeSourcePaths(candidates)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("at least one library source is required")
	}
	// Redundant parent/child roots inside one logical library remain invalid.
	// They would scan the same content twice and make relocation ambiguous.
	for i := range paths {
		for j := i + 1; j < len(paths); j++ {
			if sameOrInside(paths[i], paths[j]) || sameOrInside(paths[j], paths[i]) {
				return fmt.Errorf("library sources overlap: %s and %s", paths[i], paths[j])
			}
		}
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT ls.library_id,l.name,ls.path
FROM library_sources ls
JOIN libraries l ON l.id=ls.library_id
WHERE ls.library_id<>?
UNION ALL
SELECT l.id,l.name,l.path
FROM libraries l
WHERE l.id<>?
  AND NOT EXISTS (SELECT 1 FROM library_sources own WHERE own.library_id=l.id)`, ignoreID, ignoreID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var libraryID int64
		var name, existing string
		if err := rows.Scan(&libraryID, &name, &existing); err != nil {
			return err
		}
		existing, err = filepath.Abs(existing)
		if err != nil {
			continue
		}
		existing = filepath.Clean(existing)
		for _, candidate := range paths {
			// Exact duplicate roots across libraries are ambiguous and remain
			// forbidden. Parent/child roots are safe: the scanner assigns every
			// file to the most-specific configured source root.
			if filepath.Clean(candidate) == existing {
				return fmt.Errorf("source %s is already used by library %q", candidate, name)
			}
		}
	}
	return rows.Err()
}

func (s *Service) CreateMulti(ctx context.Context, name, kind string, paths []string, enabled bool) (ManagedLibrary, error) {
	name = strings.TrimSpace(name)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if name == "" {
		return ManagedLibrary{}, errors.New("library name is required")
	}
	if kind == "" {
		kind = "movies"
	}
	cleanPaths, err := normalizeSourcePaths(paths)
	if err != nil {
		return ManagedLibrary{}, err
	}
	if len(cleanPaths) == 0 {
		return ManagedLibrary{}, errors.New("at least one library source is required")
	}
	if err := s.ValidateLibraryPaths(ctx, 0, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagedLibrary{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO libraries(name,kind,path,enabled) VALUES(?,?,?,?)`, name, kind, cleanPaths[0], enabled)
	if err != nil {
		return ManagedLibrary{}, fmt.Errorf("create library: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ManagedLibrary{}, err
	}
	if err := replaceSourcesTx(ctx, tx, id, cleanPaths); err != nil {
		return ManagedLibrary{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedLibrary{}, err
	}
	return s.ManagedGet(ctx, id)
}

func (s *Service) AdminUpdateMulti(ctx context.Context, id int64, name, kind string, paths []string, enabled bool) (ManagedLibrary, error) {
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

func replaceSourcesTx(ctx context.Context, tx *sql.Tx, libraryID int64, paths []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_sources WHERE library_id=?`, libraryID); err != nil {
		return err
	}
	for i, path := range paths {
		label := fmt.Sprintf("Origem %d", i+1)
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_sources(library_id,path,label,enabled,sort_order) VALUES(?,?,?,?,?)`, libraryID, path, label, true, i); err != nil {
			return fmt.Errorf("save library source %s: %w", path, err)
		}
	}
	return nil
}

func markRemovedSourcesUnavailable(ctx context.Context, tx *sql.Tx, libraryID int64, oldSources []LibrarySource, newPaths []string) error {
	keep := map[string]bool{}
	for _, path := range newPaths {
		keep[filepath.Clean(path)] = true
	}
	removed := []string{}
	for _, source := range oldSources {
		root := filepath.Clean(source.Path)
		if !keep[root] {
			removed = append(removed, root)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,path FROM media WHERE library_id=? AND available=1`, libraryID)
	if err != nil {
		return err
	}
	type mediaPath struct {
		id   int64
		path string
	}
	items := []mediaPath{}
	for rows.Next() {
		var item mediaPath
		if err := rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		for _, root := range removed {
			if sameOrInside(filepath.Clean(item.path), root) {
				if _, err := tx.ExecContext(ctx, `UPDATE media SET available=0,updated_at=CURRENT_TIMESTAMP WHERE id=?`, item.id); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func (s *Service) ScanMulti(ctx context.Context, libraryID int64) (MultiScanResult, error) {
	started := time.Now()
	lib, err := s.Get(ctx, libraryID)
	if err != nil {
		return MultiScanResult{}, err
	}
	sources, err := s.scanSources(ctx, libraryID)
	if err != nil {
		return MultiScanResult{}, err
	}
	if len(sources) == 0 && strings.TrimSpace(lib.Path) != "" {
		sources = []LibrarySource{{LibraryID: libraryID, Path: lib.Path, Label: "Origem 1", Enabled: true}}
	}

	filesByPath := map[string]discoveredFile{}
	configuredRoots := []string{}
	scannedRoots := []string{}
	offline := 0
	enabledSources := 0
	for _, source := range sources {
		if source.Enabled {
			enabledSources++
		}
	}
	if enabledSources == 0 {
		return MultiScanResult{}, errors.New("library has no enabled sources")
	}

	position := 0
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		position++
		root := filepath.Clean(source.Path)
		configuredRoots = append(configuredRoots, root)
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("origem %d/%d · abrindo %s", position, enabledSources, root), libraryID)

		// Do not gate scans on os.Stat. On rclone/FUSE it is possible for stat to
		// be slow while directory listing is healthy. discover() performs the real
		// bounded read and becomes the source-of-truth for scan health. It also
		// excludes more-specific roots delegated to another logical library.
		sourceLibrary := lib
		sourceLibrary.Path = root
		discovered, discoverErr := s.discover(ctx, sourceLibrary, libraryID)
		if discoverErr != nil {
			if ctx.Err() != nil {
				return MultiScanResult{}, ctx.Err()
			}
			offline++
			_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("origem %d/%d indisponível: %v · catálogo preservado", position, enabledSources, discoverErr), libraryID)
			continue
		}
		if len(discovered) == 0 {
			previous, countErr := s.existingAvailableUnderRoot(ctx, libraryID, root)
			if countErr == nil && previous > 0 {
				offline++
				_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("origem %d/%d retornou 0 arquivos, mas antes tinha %d; catálogo preservado", position, enabledSources, previous), libraryID)
				continue
			}
		}
		scannedRoots = append(scannedRoots, root)
		for _, file := range discovered {
			filesByPath[filepath.Clean(file.path)] = file
		}
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("origem %d/%d concluída · %d arquivos encontrados", position, enabledSources, len(discovered)), libraryID)
	}
	if len(scannedRoots) == 0 {
		return MultiScanResult{}, errors.New("nenhuma origem respondeu ao scan; catálogo anterior preservado")
	}
	if err := s.gate.Wait(ctx, "media_scan_commit", func(paused bool) {
		message := "salvando catálogo"
		if paused {
			message = "Pausado antes de salvar catálogo para priorizar reprodução ativa"
		}
		_, _ = s.db.Exec(`UPDATE scan_jobs SET message=?,updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status IN ('running','cancelling')`, message, libraryID)
	}); err != nil {
		return MultiScanResult{}, err
	}

	_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("salvando catálogo · %d arquivos encontrados", len(filesByPath)), libraryID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MultiScanResult{}, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id,path FROM media WHERE library_id=?`, libraryID)
	if err != nil {
		return MultiScanResult{}, err
	}
	type existingMedia struct {
		id   int64
		path string
	}
	existing := []existingMedia{}
	for rows.Next() {
		var item existingMedia
		if err := rows.Scan(&item.id, &item.path); err != nil {
			_ = rows.Close()
			return MultiScanResult{}, err
		}
		existing = append(existing, item)
	}
	if err := rows.Close(); err != nil {
		return MultiScanResult{}, err
	}

	for _, item := range existing {
		clean := filepath.Clean(item.path)
		if _, found := filesByPath[clean]; found {
			continue
		}
		underConfigured := underAnyRoot(clean, configuredRoots)
		underScanned := underAnyRoot(clean, scannedRoots)
		if underScanned || !underConfigured {
			if _, err := tx.ExecContext(ctx, `UPDATE media SET available=0,updated_at=CURRENT_TIMESTAMP WHERE id=?`, item.id); err != nil {
				return MultiScanResult{}, err
			}
		}
	}

	for _, file := range filesByPath {
		_, err = tx.ExecContext(ctx, `
INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available)
VALUES(?,?,?,?,?,?,1)
ON CONFLICT(library_id,path) DO UPDATE SET
    extension=excluded.extension,
    size_bytes=CASE WHEN excluded.size_bytes>0 THEN excluded.size_bytes ELSE media.size_bytes END,
    modified_unix=CASE WHEN excluded.modified_unix>0 THEN excluded.modified_unix ELSE media.modified_unix END,
    available=1,
    updated_at=CURRENT_TIMESTAMP`,
			libraryID, file.title, file.path, file.extension, file.sizeBytes, file.modifiedUnix)
		if err != nil {
			return MultiScanResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return MultiScanResult{}, err
	}
	return MultiScanResult{LibraryID: libraryID, Files: len(filesByPath), DurationMS: time.Since(started).Milliseconds(), SourcesScanned: len(scannedRoots), SourcesOffline: offline}, nil
}

func (s *Service) existingAvailableUnderRoot(ctx context.Context, libraryID int64, root string) (int, error) {
	delegatedRoots, err := s.delegatedSourceRoots(ctx, libraryID, root)
	if err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM media WHERE library_id=? AND available=1`, libraryID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	root = filepath.Clean(root)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, err
		}
		path = filepath.Clean(path)
		if sameOrInside(path, root) && !underAnyRoot(path, delegatedRoots) {
			count++
		}
	}
	return count, rows.Err()
}

func normalizeSourcePaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			return nil, fmt.Errorf("resolve library source %s: %w", value, err)
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out, nil
}

func onlineSourceCount(paths []string) int {
	count := 0
	for _, path := range paths {
		if dirOnline(path) {
			count++
		}
	}
	return count
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if sameOrInside(path, root) {
			return true
		}
	}
	return false
}
