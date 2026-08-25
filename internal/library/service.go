package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Library struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type ScanResult struct {
	LibraryID  int64 `json:"library_id"`
	Files      int   `json:"files"`
	DurationMS int64 `json:"duration_ms"`
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Bootstrap(name, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM libraries`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	_, err := s.Create(context.Background(), name, "movies", path)
	return err
}

func (s *Service) Create(ctx context.Context, name, kind, path string) (Library, error) {
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(strings.ToLower(kind))
	path = strings.TrimSpace(path)

	if name == "" {
		return Library{}, errors.New("library name is required")
	}
	if path == "" {
		return Library{}, errors.New("library path is required")
	}
	if kind == "" {
		kind = "movies"
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return Library{}, fmt.Errorf("resolve library path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Library{}, fmt.Errorf("access library path: %w", err)
	}
	if !info.IsDir() {
		return Library{}, errors.New("library path must be a directory")
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO libraries(name, kind, path) VALUES(?, ?, ?)`, name, kind, abs)
	if err != nil {
		return Library{}, fmt.Errorf("create library: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Library{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id int64) (Library, error) {
	var item Library
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, kind, path, created_at, updated_at FROM libraries WHERE id = ?`, id,
	).Scan(&item.ID, &item.Name, &item.Kind, &item.Path, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) List(ctx context.Context) ([]Library, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, kind, path, created_at, updated_at FROM libraries ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Library
	for rows.Next() {
		var item Library
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Path, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Scan(ctx context.Context, libraryID int64) (ScanResult, error) {
	started := time.Now()
	lib, err := s.Get(ctx, libraryID)
	if err != nil {
		return ScanResult{}, err
	}

	type discoveredFile struct {
		path         string
		title        string
		extension    string
		sizeBytes    int64
		modifiedUnix int64
	}

	files := make([]discoveredFile, 0, 256)
	walkErr := filepath.WalkDir(lib.Path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isVideo(path) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, discoveredFile{
			path:         path,
			title:        titleFromFilename(path),
			extension:    strings.ToLower(filepath.Ext(path)),
			sizeBytes:    info.Size(),
			modifiedUnix: info.ModTime().Unix(),
		})
		return nil
	})
	if walkErr != nil {
		return ScanResult{}, fmt.Errorf("scan library: %w", walkErr)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScanResult{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE media SET available = 0 WHERE library_id = ?`, libraryID); err != nil {
		return ScanResult{}, err
	}

	for _, file := range files {
		_, err = tx.ExecContext(ctx, `
INSERT INTO media(library_id, title, path, extension, size_bytes, modified_unix, available)
VALUES(?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(library_id, path) DO UPDATE SET
    title = excluded.title,
    extension = excluded.extension,
    size_bytes = excluded.size_bytes,
    modified_unix = excluded.modified_unix,
    available = 1,
    updated_at = CURRENT_TIMESTAMP`,
			libraryID, file.title, file.path, file.extension, file.sizeBytes, file.modifiedUnix)
		if err != nil {
			return ScanResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return ScanResult{}, err
	}

	return ScanResult{LibraryID: libraryID, Files: len(files), DurationMS: time.Since(started).Milliseconds()}, nil
}

func SupportedExtensions() []string {
	exts := make([]string, 0, len(videoExtensions))
	for ext := range videoExtensions {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

var videoExtensions = map[string]struct{}{
	".mp4": {}, ".mkv": {}, ".m4v": {}, ".webm": {}, ".mov": {},
	".avi": {}, ".ts": {}, ".m2ts": {}, ".mpg": {}, ".mpeg": {},
}

func isVideo(path string) bool {
	_, ok := videoExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func titleFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Join(strings.Fields(name), " ")
}
