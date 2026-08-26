package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ManagedLibrary struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Path           string          `json:"path"`
	Paths          []string        `json:"paths"`
	Sources        []LibrarySource `json:"sources"`
	SourceCount    int             `json:"source_count"`
	OnlineSources  int             `json:"online_sources"`
	OfflineSources int             `json:"offline_sources"`
	Enabled        bool            `json:"enabled"`
	LastScanAt     *string         `json:"last_scan_at"`
	LastScanStatus string          `json:"last_scan_status"`
	LastError      string          `json:"last_error"`
	Online         bool            `json:"online"`
	MediaCount     int64           `json:"media_count"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

func (s *Service) ManagedList(ctx context.Context) ([]ManagedLibrary, error) {
	_, _ = s.RecoverStaleScans(ctx, 5*time.Minute)
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,l.name,l.kind,l.path,l.enabled,l.last_scan_at,l.last_scan_status,l.last_error,l.created_at,l.updated_at,(SELECT COUNT(*) FROM media m WHERE m.library_id=l.id AND m.available=1) FROM libraries l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	out := []ManagedLibrary{}
	for rows.Next() {
		var v ManagedLibrary
		if err := rows.Scan(&v.ID, &v.Name, &v.Kind, &v.Path, &v.Enabled, &v.LastScanAt, &v.LastScanStatus, &v.LastError, &v.CreatedAt, &v.UpdatedAt, &v.MediaCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.enrichManagedSources(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) ManagedGet(ctx context.Context, id int64) (ManagedLibrary, error) {
	var v ManagedLibrary
	err := s.db.QueryRowContext(ctx, `SELECT l.id,l.name,l.kind,l.path,l.enabled,l.last_scan_at,l.last_scan_status,l.last_error,l.created_at,l.updated_at,(SELECT COUNT(*) FROM media m WHERE m.library_id=l.id AND m.available=1) FROM libraries l WHERE l.id=?`, id).Scan(&v.ID, &v.Name, &v.Kind, &v.Path, &v.Enabled, &v.LastScanAt, &v.LastScanStatus, &v.LastError, &v.CreatedAt, &v.UpdatedAt, &v.MediaCount)
	if err != nil {
		return v, err
	}
	if err := s.enrichManagedSources(ctx, &v); err != nil {
		return ManagedLibrary{}, err
	}
	return v, nil
}

func (s *Service) enrichManagedSources(ctx context.Context, v *ManagedLibrary) error {
	sources, err := s.Sources(ctx, v.ID)
	if err != nil {
		return err
	}
	if len(sources) == 0 && strings.TrimSpace(v.Path) != "" {
		sources = []LibrarySource{{LibraryID: v.ID, Path: v.Path, Label: "Origem 1", Enabled: true, Online: dirOnline(v.Path)}}
	}
	v.Sources = sources
	v.Paths = make([]string, 0, len(sources))
	v.OnlineSources = 0
	for _, source := range sources {
		v.Paths = append(v.Paths, source.Path)
		if source.Online {
			v.OnlineSources++
		}
	}
	v.SourceCount = len(sources)
	v.OfflineSources = v.SourceCount - v.OnlineSources
	v.Online = v.OnlineSources > 0
	if len(v.Paths) > 0 {
		v.Path = v.Paths[0]
	}
	return nil
}

func (s *Service) AdminUpdate(ctx context.Context, id int64, name, kind, path string, enabled bool) (ManagedLibrary, error) {
	return s.AdminUpdateMulti(ctx, id, name, kind, []string{path}, enabled)
}

func (s *Service) AdminDelete(ctx context.Context, id int64) error {
	s.scanMu.Lock()
	if s.running[id] {
		s.scanMu.Unlock()
		return errors.New("cannot remove a library while a scan is running")
	}
	s.scanMu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// StartAdminScan validates at least one source and starts a bounded background scan.
// Offline sources are preserved instead of being interpreted as deleted media.
func (s *Service) StartAdminScan(ctx context.Context, id int64) (ManagedLibrary, error) {
	v, err := s.ManagedGet(ctx, id)
	if err != nil {
		return ManagedLibrary{}, err
	}
	if !v.Enabled {
		return ManagedLibrary{}, errors.New("library is disabled")
	}
	if v.OnlineSources == 0 {
		msg := "all library sources are offline; catalog preserved"
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='offline',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, msg, id)
		return ManagedLibrary{}, errors.New(msg)
	}

	s.scanMu.Lock()
	if s.running[id] {
		s.scanMu.Unlock()
		return ManagedLibrary{}, errors.New("a library scan is already running")
	}
	scanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.running[id] = true
	s.scanCancel[id] = cancel
	s.scanMu.Unlock()

	queued := fmt.Sprintf("queued · %d/%d sources online", v.OnlineSources, v.SourceCount)
	if _, err := s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='running',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, queued, id); err != nil {
		cancel()
		s.clearScan(id)
		return ManagedLibrary{}, err
	}

	go s.runAdminScan(id, scanCtx, cancel)
	v.LastScanStatus = "running"
	v.LastError = queued
	return v, nil
}

func (s *Service) runAdminScan(id int64, ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	defer s.clearScan(id)
	go s.watchScanProgress(id, ctx, cancel, 3*time.Minute)

	result, err := s.ScanMulti(ctx, id)
	if err != nil {
		var currentStatus string
		_ = s.db.QueryRow(`SELECT last_scan_status FROM libraries WHERE id=?`, id).Scan(&currentStatus)
		if currentStatus == "timeout" {
			return
		}
		status := "error"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			status = "timeout"
		} else if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			status = "cancelled"
		} else if strings.Contains(strings.ToLower(err.Error()), "offline") {
			status = "offline"
		}
		message := err.Error()
		if status == "cancelled" {
			message = "scan cancelled by administrator; previous catalog preserved"
		}
		_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status=?,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, id)
		return
	}
	status := "ok"
	message := fmt.Sprintf("%d files · %d/%d sources scanned · %d ms", result.Files, result.SourcesScanned, result.SourcesScanned+result.SourcesOffline, result.DurationMS)
	if result.SourcesOffline > 0 {
		status = "partial"
		message += fmt.Sprintf(" · %d source(s) offline/suspicious preserved", result.SourcesOffline)
	}
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_at=CURRENT_TIMESTAMP,last_scan_status=?,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, id)
}

func (s *Service) CancelAdminScan(ctx context.Context, id int64) error {
	s.scanMu.Lock()
	running := s.running[id]
	cancel := s.scanCancel[id]
	s.scanMu.Unlock()
	if !running || cancel == nil {
		var status string
		if err := s.db.QueryRowContext(ctx, `SELECT last_scan_status FROM libraries WHERE id=?`, id).Scan(&status); err != nil {
			return err
		}
		if status != "running" && status != "cancelling" {
			return errors.New("no library scan is running")
		}
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='cancelling',last_error='cancel requested; stopping scan safely',updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	if cancel != nil {
		cancel()
	}
	return nil
}

// RecoverStaleScans prevents a scan status from living forever if a remote
// filesystem stops making progress. Active scans refresh libraries.updated_at.
func (s *Service) RecoverStaleScans(ctx context.Context, maxIdle time.Duration) (int64, error) {
	if maxIdle <= 0 {
		maxIdle = 5 * time.Minute
	}
	seconds := int64(maxIdle.Seconds())
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM libraries WHERE last_scan_status IN ('running','cancelling') AND (strftime('%s','now')-strftime('%s',updated_at))>?`, seconds)
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		s.scanMu.Lock()
		cancel := s.scanCancel[id]
		s.scanMu.Unlock()
		if cancel != nil {
			cancel()
		}
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='timeout',last_error='scan stopped reporting progress and was recovered automatically',updated_at=CURRENT_TIMESTAMP WHERE id=? AND last_scan_status IN ('running','cancelling')`, id)
	}
	return int64(len(ids)), nil
}

// AdminScan remains available for internal/tests that need a synchronous scan.
func (s *Service) AdminScan(ctx context.Context, id int64) (ScanResult, error) {
	v, err := s.ManagedGet(ctx, id)
	if err != nil {
		return ScanResult{}, err
	}
	if !v.Enabled {
		return ScanResult{}, errors.New("library is disabled")
	}
	if v.OnlineSources == 0 {
		msg := "all library sources are offline; catalog preserved"
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='offline',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, msg, id)
		return ScanResult{}, errors.New(msg)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='running',last_error='',updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	result, err := s.ScanMulti(ctx, id)
	if err != nil {
		_, _ = s.db.ExecContext(context.Background(), `UPDATE libraries SET last_scan_status='error',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, err.Error(), id)
		return ScanResult{}, err
	}
	status := "ok"
	message := ""
	if result.SourcesOffline > 0 {
		status = "partial"
		message = fmt.Sprintf("%d source(s) offline preserved", result.SourcesOffline)
	}
	_, _ = s.db.ExecContext(context.Background(), `UPDATE libraries SET last_scan_at=CURRENT_TIMESTAMP,last_scan_status=?,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, id)
	return ScanResult{LibraryID: result.LibraryID, Files: result.Files, DurationMS: result.DurationMS}, nil
}

func (s *Service) clearScan(id int64) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	delete(s.running, id)
	delete(s.scanCancel, id)
}

func (s *Service) setScanRunning(id int64, value bool) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if value {
		s.running[id] = true
	} else {
		delete(s.running, id)
		delete(s.scanCancel, id)
	}
}

func dirOnline(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	type statResult struct {
		info os.FileInfo
		err  error
	}
	ch := make(chan statResult, 1)
	go func() {
		info, err := os.Stat(path)
		ch <- statResult{info: info, err: err}
	}()
	select {
	case <-ctx.Done():
		return false
	case result := <-ch:
		return result.err == nil && result.info.IsDir()
	}
}

func cleanLibraryPath(path string) string {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return strings.TrimSpace(path)
	}
	return filepath.Clean(abs)
}
