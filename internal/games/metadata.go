package games

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type MetadataJob struct {
	ID         int64   `json:"id"`
	LibraryID  *int64  `json:"library_id"`
	Library    string  `json:"library"`
	Status     string  `json:"status"`
	Progress   int     `json:"progress"`
	Total      int     `json:"total"`
	Processed  int     `json:"processed"`
	Matched    int     `json:"matched"`
	Failed     int     `json:"failed"`
	Provider   string  `json:"provider"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type gameMetadataCandidate struct {
	Provider      string
	ProviderID    string
	Title         string
	Overview      string
	ReleaseYear   int
	Genres        []string
	Developers    []string
	Publishers    []string
	Rating        float64
	CoverURL      string
	Screenshots   []string
	SteamGridDBID string
}

type metadataGameRow struct {
	ID       int64
	Library  int64
	Platform string
	Title    string
	Hash     string
}

var gameMetadataWorkers sync.Map // *Service -> bool

func (s *Service) EnqueueMetadata(ctx context.Context, libraryID int64, refresh bool) (MetadataJob, error) {
	if libraryID > 0 {
		var kind string
		if err := s.db.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=? AND enabled=1`, libraryID).Scan(&kind); err != nil {
			return MetadataJob{}, err
		}
		if !strings.EqualFold(strings.TrimSpace(kind), "games") {
			return MetadataJob{}, errors.New("library is not a games library")
		}
	}

	providers, err := s.ProviderSettings(ctx)
	if err != nil {
		return MetadataJob{}, err
	}
	active := make([]string, 0, len(providers))
	identificationReady := false
	for _, provider := range providers {
		if !provider.Enabled || !provider.Configured || !metadataProviderRuntimeSupported(provider.Key) {
			continue
		}
		active = append(active, provider.Key)
		switch provider.Key {
		case "screenscraper", "igdb", "mobygames", "thegamesdb", "hasheous":
			identificationReady = true
		}
	}
	if !identificationReady {
		return MetadataJob{}, errors.New("configure and enable ScreenScraper, IGDB, MobyGames, TheGamesDB or Hasheous first")
	}

	var existing int64
	if libraryID > 0 {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM game_metadata_jobs WHERE library_id=? AND status IN ('queued','running') ORDER BY id DESC LIMIT 1`, libraryID).Scan(&existing)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM game_metadata_jobs WHERE library_id IS NULL AND status IN ('queued','running') ORDER BY id DESC LIMIT 1`).Scan(&existing)
	}
	if err == nil {
		return s.MetadataJob(ctx, existing)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MetadataJob{}, err
	}

	providerLabel := strings.Join(active, ",")
	message := "metadados pendentes aguardando na fila"
	if refresh {
		message = "atualização completa aguardando na fila"
		providerLabel += ":refresh"
	}
	var result sql.Result
	if libraryID > 0 {
		result, err = s.db.ExecContext(ctx, `INSERT INTO game_metadata_jobs(library_id,status,provider,message) VALUES(?,'queued',?,?)`, libraryID, providerLabel, message)
	} else {
		result, err = s.db.ExecContext(ctx, `INSERT INTO game_metadata_jobs(library_id,status,provider,message) VALUES(NULL,'queued',?,?)`, providerLabel, message)
	}
	if err != nil {
		return MetadataJob{}, err
	}
	id, _ := result.LastInsertId()
	go s.drainMetadata()
	return s.MetadataJob(ctx, id)
}

func (s *Service) MetadataJob(ctx context.Context, id int64) (MetadataJob, error) {
	var job MetadataJob
	err := s.db.QueryRowContext(ctx, `SELECT j.id,j.library_id,COALESCE(l.name,'Todos os Games'),j.status,j.progress,j.total,j.processed,j.matched,j.failed,j.provider,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM game_metadata_jobs j LEFT JOIN libraries l ON l.id=j.library_id WHERE j.id=?`, id).
		Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Provider, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return job, err
}

func (s *Service) MetadataJobs(ctx context.Context, limit int) ([]MetadataJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.library_id,COALESCE(l.name,'Todos os Games'),j.status,j.progress,j.total,j.processed,j.matched,j.failed,j.provider,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM game_metadata_jobs j LEFT JOIN libraries l ON l.id=j.library_id ORDER BY CASE j.status WHEN 'running' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END,j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetadataJob{}
	for rows.Next() {
		var job MetadataJob
		if err := rows.Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Progress, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Provider, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	go s.drainMetadata()
	return out, rows.Err()
}

func (s *Service) drainMetadata() {
	if _, loaded := gameMetadataWorkers.LoadOrStore(s, true); loaded {
		return
	}
	defer gameMetadataWorkers.Delete(s)
	for {
		var jobID int64
		var libraryID sql.NullInt64
		var provider string
		err := s.db.QueryRow(`SELECT id,library_id,provider FROM game_metadata_jobs WHERE status='queued' ORDER BY id LIMIT 1`).Scan(&jobID, &libraryID, &provider)
		if err != nil {
			return
		}
		result, err := s.db.Exec(`UPDATE game_metadata_jobs SET status='running',progress=1,started_at=CURRENT_TIMESTAMP,message='preparando catálogo de Games',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, jobID)
		if err != nil {
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			continue
		}
		var lib int64
		if libraryID.Valid {
			lib = libraryID.Int64
		}
		s.runMetadata(context.Background(), jobID, lib, strings.Contains(provider, ":refresh"))
	}
}

func (s *Service) runMetadata(ctx context.Context, jobID, libraryID int64, refresh bool) {
	games, err := s.metadataCandidates(ctx, libraryID, refresh)
	if err != nil {
		s.finishMetadataJob(jobID, 0, 0, 1, "error", err.Error())
		return
	}
	_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET total=?,progress=3,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, len(games), fmt.Sprintf("%d jogo(s) para enriquecer", len(games)), jobID)
	if len(games) == 0 {
		s.finishMetadataJob(jobID, 0, 0, 0, "completed", "nenhum metadata pendente")
		return
	}

	processed, matched, failed := 0, 0, 0
	for _, game := range games {
		for s.playbackBusy(ctx) {
			_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET message='Pausado para priorizar reprodução/jogo ativo',updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
			select {
			case <-ctx.Done():
				s.finishMetadataJob(jobID, processed, matched, failed+1, "error", "metadata interrompido")
				return
			case <-time.After(20 * time.Second):
			}
		}

		provider, candidate, candidateErr := s.enrichGame(ctx, game)
		processed++
		if candidateErr != nil {
			failed++
			_ = s.storeMetadataError(ctx, game.ID, candidateErr)
		} else if candidate != nil {
			if err := s.storeMetadataCandidate(ctx, game, *candidate); err != nil {
				failed++
			} else {
				matched++
			}
		}

		progress := 3 + processed*96/len(games)
		if progress > 99 {
			progress = 99
		}
		message := fmt.Sprintf("%d/%d · %s", processed, len(games), game.Title)
		if provider != "" {
			message += " · " + provider
		}
		_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET progress=?,processed=?,matched=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, progress, processed, matched, failed, message, jobID)

		// The slowest provider is deliberately allowed breathing room. This also
		// keeps background enrichment friendly to remote/rclone-backed libraries.
		select {
		case <-ctx.Done():
			return
		case <-time.After(1100 * time.Millisecond):
		}
	}

	status := "completed"
	if failed > 0 {
		status = "completed_with_errors"
	}
	s.finishMetadataJob(jobID, processed, matched, failed, status, fmt.Sprintf("%d/%d jogo(s) enriquecido(s) · %d erro(s)", matched, processed, failed))
}

func (s *Service) metadataCandidates(ctx context.Context, libraryID int64, refresh bool) ([]metadataGameRow, error) {
	where := ` WHERE COALESCE(gm.metadata_locked,0)=0`
	args := []any{}
	if libraryID > 0 {
		where += ` AND g.library_id=?`
		args = append(args, libraryID)
	}
	if !refresh {
		where += ` AND (gm.game_id IS NULL OR gm.refreshed_at IS NULL OR gm.last_error<>'')`
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.id,g.library_id,g.platform,g.title,g.content_hash FROM games g LEFT JOIN game_metadata gm ON gm.game_id=g.id`+where+` ORDER BY g.library_id,g.platform,g.sort_title,g.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []metadataGameRow{}
	for rows.Next() {
		var game metadataGameRow
		if err := rows.Scan(&game.ID, &game.Library, &game.Platform, &game.Title, &game.Hash); err != nil {
			return nil, err
		}
		out = append(out, game)
	}
	return out, rows.Err()
}

func (s *Service) playbackBusy(ctx context.Context) bool {
	var video, games int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_sessions WHERE last_seen_at>=datetime('now','-90 seconds')`).Scan(&video)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_play_sessions WHERE last_seen_at>=datetime('now','-45 seconds')`).Scan(&games)
	return video+games > 0
}

func (s *Service) enrichGame(ctx context.Context, game metadataGameRow) (string, *gameMetadataCandidate, error) {
	return s.enrichGameStackV2(ctx, game)
}
