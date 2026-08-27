package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type ScanJob struct {
	ID             int64   `json:"id"`
	LibraryID      int64   `json:"library_id"`
	Library        string  `json:"library"`
	Status         string  `json:"status"`
	Progress       int     `json:"progress"`
	Files          int     `json:"files"`
	SourcesTotal   int     `json:"sources_total"`
	SourcesScanned int     `json:"sources_scanned"`
	SourcesOffline int     `json:"sources_offline"`
	Message        string  `json:"message"`
	CreatedAt      string  `json:"created_at"`
	StartedAt      *string `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
	UpdatedAt      string  `json:"updated_at"`
}

// EnqueueAdminScan adds one scan to the persistent FIFO. Scans are intentionally
// serialized: cloud mounts and SQLite writes stay predictable even when an
// operator asks to scan every library at once.
func (s *Service) EnqueueAdminScan(ctx context.Context, id int64) (ManagedLibrary, ScanJob, error) {
	v, err := s.ManagedGet(ctx, id)
	if err != nil {
		return ManagedLibrary{}, ScanJob{}, err
	}
	if !v.Enabled {
		return ManagedLibrary{}, ScanJob{}, errors.New("library is disabled")
	}
	if v.SourceCount == 0 {
		return ManagedLibrary{}, ScanJob{}, errors.New("library has no configured sources")
	}
	var existingID int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM scan_jobs WHERE library_id=? AND status IN ('queued','running','cancelling') ORDER BY id DESC LIMIT 1`, id).Scan(&existingID)
	if err == nil {
		job, getErr := s.ScanJob(ctx, existingID)
		if getErr != nil {
			return ManagedLibrary{}, ScanJob{}, getErr
		}
		return v, job, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ManagedLibrary{}, ScanJob{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO scan_jobs(library_id,status,progress,sources_total,message) VALUES(?,'queued',0,?,'aguardando na fila')`, id, v.SourceCount)
	if err != nil {
		return ManagedLibrary{}, ScanJob{}, err
	}
	jobID, _ := res.LastInsertId()
	position := s.scanQueuePosition(ctx, jobID)
	message := fmt.Sprintf("na fila · posição %d · %d origem(ns)", position, v.SourceCount)
	_, _ = s.db.ExecContext(ctx, `UPDATE scan_jobs SET message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, jobID)
	_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='queued',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, id)
	job, err := s.ScanJob(ctx, jobID)
	if err != nil {
		return ManagedLibrary{}, ScanJob{}, err
	}
	go s.drainScanQueue()
	v.LastScanStatus = "queued"
	v.LastError = message
	return v, job, nil
}

// EnqueueAllAdminScans builds the complete batch atomically before starting the
// scan worker. This is important for SQLite: the first scan must not begin a
// catalog write transaction while the same request is still inserting the rest
// of the queue.
func (s *Service) EnqueueAllAdminScans(ctx context.Context) ([]ScanJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.id,l.name,COUNT(CASE WHEN ls.enabled=1 THEN 1 END) FROM libraries l LEFT JOIN library_sources ls ON ls.library_id=l.id WHERE l.enabled=1 GROUP BY l.id,l.name ORDER BY l.name,l.id`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id          int64
		name        string
		sourceCount int
	}
	candidates := []candidate{}
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.name, &item.sourceCount); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if item.sourceCount > 0 {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	jobIDs := []int64{}
	for _, item := range candidates {
		var existingID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM scan_jobs WHERE library_id=? AND status IN ('queued','running','cancelling') ORDER BY id DESC LIMIT 1`, item.id).Scan(&existingID)
		if err == nil {
			jobIDs = append(jobIDs, existingID)
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		res, insertErr := tx.ExecContext(ctx, `INSERT INTO scan_jobs(library_id,status,progress,sources_total,message) VALUES(?,'queued',0,?,'aguardando na fila')`, item.id, item.sourceCount)
		if insertErr != nil {
			return nil, insertErr
		}
		jobID, _ := res.LastInsertId()
		jobIDs = append(jobIDs, jobID)
		message := fmt.Sprintf("na fila · lote Escanear todas · %d origem(ns)", item.sourceCount)
		if _, err := tx.ExecContext(ctx, `UPDATE libraries SET last_scan_status='queued',last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, item.id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE scan_jobs SET message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, message, jobID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	jobs := make([]ScanJob, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		job, getErr := s.ScanJob(ctx, jobID)
		if getErr == nil {
			jobs = append(jobs, job)
		}
	}
	go s.drainScanQueue()
	return jobs, nil
}

func (s *Service) ScanJob(ctx context.Context, id int64) (ScanJob, error) {
	var job ScanJob
	err := s.db.QueryRowContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.progress,j.files,j.sources_total,j.sources_scanned,j.sources_offline,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM scan_jobs j JOIN libraries l ON l.id=j.library_id WHERE j.id=?`, id).
		Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Files, &job.SourcesTotal, &job.SourcesScanned, &job.SourcesOffline, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return job, err
}

func (s *Service) ScanJobs(ctx context.Context, limit int) ([]ScanJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.progress,j.files,j.sources_total,j.sources_scanned,j.sources_offline,CASE WHEN j.status='running' AND l.last_error<>'' THEN l.last_error ELSE j.message END,j.created_at,j.started_at,j.finished_at,j.updated_at FROM scan_jobs j JOIN libraries l ON l.id=j.library_id ORDER BY CASE j.status WHEN 'running' THEN 0 WHEN 'cancelling' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []ScanJob{}
	for rows.Next() {
		var job ScanJob
		if err := rows.Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Files, &job.SourcesTotal, &job.SourcesScanned, &job.SourcesOffline, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	go s.drainScanQueue()
	return jobs, rows.Err()
}

func (s *Service) CancelQueuedOrRunningAdminScan(ctx context.Context, libraryID int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE scan_jobs SET status='cancelled',message='removido da fila pelo administrador',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status='queued'`, libraryID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		_, _ = s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_status='cancelled',last_error='scan removido da fila; catálogo preservado',updated_at=CURRENT_TIMESTAMP WHERE id=? AND last_scan_status='queued'`, libraryID)
		return nil
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE scan_jobs SET status='cancelling',message='cancelamento solicitado; parando com segurança',updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status='running'`, libraryID)
	return s.CancelAdminScan(ctx, libraryID)
}

func (s *Service) scanQueuePosition(ctx context.Context, jobID int64) int {
	var position int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scan_jobs WHERE status='queued' AND id<=?`, jobID).Scan(&position)
	if position < 1 {
		position = 1
	}
	return position
}

func (s *Service) drainScanQueue() {
	// Reuse the existing scan mutex with an impossible library id as the worker
	// guard. This keeps one FIFO worker per Service without adding another lock.
	s.scanMu.Lock()
	if s.running[0] {
		s.scanMu.Unlock()
		return
	}
	s.running[0] = true
	s.scanMu.Unlock()
	defer func() {
		s.scanMu.Lock()
		delete(s.running, 0)
		s.scanMu.Unlock()
	}()

	for {
		var jobID, libraryID int64
		err := s.db.QueryRow(`SELECT id,library_id FROM scan_jobs WHERE status='queued' ORDER BY id LIMIT 1`).Scan(&jobID, &libraryID)
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			return
		}
		scanCtx, cancel, ok := s.claimQueuedScan(jobID, libraryID)
		if !ok {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		s.runQueuedScan(jobID, libraryID, scanCtx, cancel)
	}
}

func (s *Service) claimQueuedScan(jobID, libraryID int64) (context.Context, context.CancelFunc, bool) {
	s.scanMu.Lock()
	if s.running[libraryID] {
		s.scanMu.Unlock()
		_, _ = s.db.Exec(`UPDATE scan_jobs SET message='aguardando outro scan da biblioteca terminar',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, jobID)
		return nil, nil, false
	}
	s.scanMu.Unlock()

	res, err := s.db.Exec(`UPDATE scan_jobs SET status='running',progress=3,started_at=COALESCE(started_at,CURRENT_TIMESTAMP),message='iniciando scan',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, jobID)
	if err != nil {
		return nil, nil, false
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil, false
	}
	scanCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	s.scanMu.Lock()
	// Recheck while holding the lock to avoid a direct legacy scan claiming the
	// same library between the first check and the database claim.
	if s.running[libraryID] {
		s.scanMu.Unlock()
		cancel()
		_, _ = s.db.Exec(`UPDATE scan_jobs SET status='queued',progress=0,started_at=NULL,message='aguardando outro scan da biblioteca terminar',updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
		return nil, nil, false
	}
	s.running[libraryID] = true
	s.scanCancel[libraryID] = cancel
	s.scanMu.Unlock()
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status='running',last_error='iniciando scan da fila',updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryID)
	go s.watchScanProgress(libraryID, scanCtx, cancel, 6*time.Minute)
	return scanCtx, cancel, true
}

func (s *Service) runQueuedScan(jobID, libraryID int64, ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel()
		s.clearScan(libraryID)
	}()

	result, err := s.ScanMulti(ctx, libraryID)
	if err != nil {
		status := "error"
		message := err.Error()
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			status = "cancelled"
			message = "scan cancelado pelo administrador; catálogo anterior preservado"
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			status = "timeout"
		}
		_, _ = s.db.Exec(`UPDATE scan_jobs SET status=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, jobID)
		_, _ = s.db.Exec(`UPDATE libraries SET last_scan_status=?,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, libraryID)
		return
	}
	status := "completed"
	libraryStatus := "ok"
	message := fmt.Sprintf("%d arquivos · %d/%d origens lidas · %d ms", result.Files, result.SourcesScanned, result.SourcesScanned+result.SourcesOffline, result.DurationMS)
	if result.SourcesOffline > 0 {
		status = "completed_with_errors"
		libraryStatus = "partial"
		message += fmt.Sprintf(" · %d origem(ns) indisponível(is) preservada(s)", result.SourcesOffline)
	}
	_, _ = s.db.Exec(`UPDATE scan_jobs SET status=?,progress=100,files=?,sources_scanned=?,sources_offline=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, result.Files, result.SourcesScanned, result.SourcesOffline, message, jobID)
	_, _ = s.db.Exec(`UPDATE libraries SET last_scan_at=CURRENT_TIMESTAMP,last_scan_status=?,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, libraryStatus, message, libraryID)
}

// updateActiveScanJob mirrors the already-throttled scanner status into the
// queue table. It keeps the queue UI informative without adding a second scan
// traversal or expensive polling query inside the scanner.
func (s *Service) updateActiveScanJob(ctx context.Context, libraryID int64, files, sourcesDone, sourcesTotal, sourcesOffline int, message string) {
	progress := 3
	if sourcesTotal > 0 {
		progress = 5 + (sourcesDone * 85 / sourcesTotal)
		if progress > 92 {
			progress = 92
		}
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE scan_jobs SET progress=?,files=?,sources_scanned=?,sources_offline=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE library_id=? AND status IN ('running','cancelling')`, progress, files, sourcesDone, sourcesOffline, message, libraryID)
}
