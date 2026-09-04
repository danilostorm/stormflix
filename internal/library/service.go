package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/workload"
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
	db         *sql.DB
	scanMu     sync.Mutex
	running    map[int64]bool
	scanCancel map[int64]context.CancelFunc
	gate       *workload.Gate
}

func NewService(db *sql.DB) *Service {
	s := &Service{db: db, running: map[int64]bool{}, scanCancel: map[int64]context.CancelFunc{}, gate: workload.For(db)}
	_, _ = db.Exec(`UPDATE libraries SET last_scan_status='interrupted',last_error='scan interrupted by server restart',updated_at=CURRENT_TIMESTAMP WHERE last_scan_status IN ('running','cancelling')`)
	return s
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
	result, err := s.db.ExecContext(ctx, `INSERT INTO libraries(name, kind, path) VALUES(?, ?, ?)`, name, kind, abs)
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
	err := s.db.QueryRowContext(ctx, `SELECT id, name, kind, path, created_at, updated_at FROM libraries WHERE id = ?`, id).Scan(&item.ID, &item.Name, &item.Kind, &item.Path, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) List(ctx context.Context) ([]Library, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, path, created_at, updated_at FROM libraries ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Library{}
	for rows.Next() {
		var item Library
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Path, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type discoveredFile struct {
	path         string
	title        string
	extension    string
	sizeBytes    int64
	modifiedUnix int64
}

func (s *Service) Scan(ctx context.Context, libraryID int64) (ScanResult, error) {
	started := time.Now()
	lib, err := s.Get(ctx, libraryID)
	if err != nil {
		return ScanResult{}, err
	}
	files, err := s.discover(ctx, lib, libraryID)
	if err != nil {
		return ScanResult{}, fmt.Errorf("scan library: %w", err)
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
    extension = excluded.extension,
    size_bytes = CASE WHEN excluded.size_bytes > 0 THEN excluded.size_bytes ELSE media.size_bytes END,
    modified_unix = CASE WHEN excluded.modified_unix > 0 THEN excluded.modified_unix ELSE media.modified_unix END,
    available = 1,
    updated_at = CURRENT_TIMESTAMP`, libraryID, file.title, file.path, file.extension, file.sizeBytes, file.modifiedUnix)
		if err != nil {
			return ScanResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{LibraryID: libraryID, Files: len(files), DurationMS: time.Since(started).Milliseconds()}, nil
}

func (s *Service) discover(ctx context.Context, lib Library, libraryID int64) ([]discoveredFile, error) {
	files := make([]discoveredFile, 0, 256)
	delegatedRoots, err := s.delegatedSourceRoots(ctx, libraryID, lib.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve nested library ownership: %w", err)
	}
	dirs := []string{lib.Path}
	lastProgress := time.Time{}
	statWarnings := 0
	touchProgress := func(dir string, force bool) {
		if !force && !lastProgress.IsZero() && time.Since(lastProgress) < 1500*time.Millisecond {
			return
		}
		rel, _ := filepath.Rel(lib.Path, dir)
		if rel == "." {
			rel = "/"
		}
		msg := fmt.Sprintf("lendo %s · %d arquivos encontrados", rel, len(files))
		if len(delegatedRoots) > 0 {
			msg += fmt.Sprintf(" · %d subpasta(s) delegada(s)", len(delegatedRoots))
		}
		if statWarnings > 0 {
			msg += fmt.Sprintf(" · %d stats lentos ignorados", statWarnings)
		}
		_, _ = s.db.Exec(`UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND last_scan_status IN ('running','cancelling')`, msg, libraryID)
		lastProgress = time.Now()
	}
	for len(dirs) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := s.gate.Wait(ctx, "media_scan", func(paused bool) {
			message := "retomando scan após reprodução"
			if paused {
				message = "Pausado para priorizar reprodução ativa"
			}
			_, _ = s.db.Exec(`UPDATE libraries SET last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND last_scan_status IN ('running','cancelling')`, message, libraryID)
			_, _ = s.db.Exec(`UPDATE scan_jobs SET message=?,updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status IN ('running','cancelling')`, message, libraryID)
		}); err != nil {
			return nil, err
		}
		dir := dirs[0]
		dirs = dirs[1:]
		touchProgress(dir, lastProgress.IsZero())
		entries, err := readDirContext(ctx, dir)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			path := filepath.Clean(filepath.Join(dir, entry.Name()))
			if entry.IsDir() {
				// Another library with a more-specific source root owns this subtree.
				// Do not descend into it from a broad parent library; this is the
				// scanner-side half of allowing safe parent/child library roots.
				if underAnyRoot(path, delegatedRoots) {
					touchProgress(dir, false)
					continue
				}
				dirs = append(dirs, path)
				touchProgress(dir, false)
				continue
			}
			if !isLibraryMedia(path, lib.Kind) {
				touchProgress(dir, false)
				continue
			}
			file := discoveredFile{path: path, title: titleFromFilename(path), extension: strings.ToLower(filepath.Ext(path))}
			if info, infoErr := entryInfoContext(ctx, entry); infoErr == nil {
				file.sizeBytes = info.Size()
				file.modifiedUnix = info.ModTime().Unix()
			} else if ctx.Err() != nil {
				return nil, ctx.Err()
			} else {
				statWarnings++
			}
			files = append(files, file)
			touchProgress(dir, false)
		}
		touchProgress(dir, true)
	}
	return files, nil
}

type readDirResult struct {
	entries []os.DirEntry
	err     error
}

func readDirContext(parent context.Context, path string) ([]os.DirEntry, error) {
	// Large cloud folders can legitimately take more than a minute to hydrate
	// through rclone/FUSE. Bound the call, but leave enough room for healthy
	// remotes instead of declaring them dead too early.
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	ch := make(chan readDirResult, 1)
	go func() {
		entries, err := os.ReadDir(path)
		ch <- readDirResult{entries: entries, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("directory listing timeout: %w", ctx.Err())
	case result := <-ch:
		return result.entries, result.err
	}
}

type infoResult struct {
	info os.FileInfo
	err  error
}

func entryInfoContext(parent context.Context, entry os.DirEntry) (os.FileInfo, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	ch := make(chan infoResult, 1)
	go func() {
		info, err := entry.Info()
		ch <- infoResult{info: info, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("file stat timeout: %w", ctx.Err())
	case result := <-ch:
		return result.info, result.err
	}
}

func SupportedExtensions() []string {
	exts := make([]string, 0, len(videoExtensions)+len(audioExtensions))
	for ext := range videoExtensions {
		exts = append(exts, ext)
	}
	for ext := range audioExtensions {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

var videoExtensions = map[string]struct{}{
	".mp4": {}, ".mkv": {}, ".m4v": {}, ".webm": {}, ".mov": {},
	".avi": {}, ".ts": {}, ".m2ts": {}, ".mpg": {}, ".mpeg": {},
}

var audioExtensions = map[string]struct{}{
	".mp3": {}, ".flac": {}, ".m4a": {}, ".aac": {}, ".ogg": {},
	".opus": {}, ".wav": {}, ".wma": {}, ".aiff": {}, ".aif": {},
	".ape": {}, ".mka": {},
}

func isVideo(path string) bool {
	_, ok := videoExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func isAudio(path string) bool {
	_, ok := audioExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func isLibraryMedia(path, kind string) bool {
	if strings.EqualFold(strings.TrimSpace(kind), "music") {
		return isAudio(path)
	}
	return isVideo(path)
}

func titleFromFilename(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Join(strings.Fields(name), " ")
}
