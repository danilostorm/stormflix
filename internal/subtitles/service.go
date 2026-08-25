package subtitles

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/config"
)

type Service struct {
	db        *sql.DB
	assets    *assets.Store
	providers []Provider
	mu        sync.Mutex
	running   map[int64]bool
}

type Job struct {
	ID         int64   `json:"id"`
	LibraryID  int64   `json:"library_id"`
	Library    string  `json:"library"`
	Language   string  `json:"language"`
	Status     string  `json:"status"`
	Total      int     `json:"total"`
	Processed  int     `json:"processed"`
	Downloaded int     `json:"downloaded"`
	Failed     int     `json:"failed"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type Item struct {
	ID              int64  `json:"id"`
	MediaID         int64  `json:"media_id"`
	Language        string `json:"language"`
	HearingImpaired bool   `json:"hearing_impaired"`
	Format          string `json:"format"`
	Provider        string `json:"provider"`
	ReleaseName     string `json:"release_name"`
	PublicURL       string `json:"public_url"`
	CreatedAt       string `json:"created_at"`
}

func NewService(db *sql.DB, cfg config.Config, store *assets.Store) *Service {
	s := &Service{
		db: db,
		assets: store,
		providers: []Provider{
			NewOpenSubtitlesProvider(cfg.OpenSubtitlesAPIKey, cfg.OpenSubtitlesUsername, cfg.OpenSubtitlesPassword, cfg.OpenSubtitlesUserAgent),
			NewSubDLProvider(cfg.SubDLAPIKey),
		},
		running: map[int64]bool{},
	}
	_, _ = db.Exec(`UPDATE subtitle_jobs SET status='failed',message='server restarted while job was running',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running')`)
	return s
}

func (s *Service) Agents() []AgentStatus {
	out := make([]AgentStatus, 0, len(s.providers))
	for _, provider := range s.providers {
		description := "Subtitle search and automatic download."
		switch provider.Name() {
		case "opensubtitles":
			description = "OpenSubtitles REST API: primary subtitle search/download agent."
		case "subdl":
			description = "SubDL: fallback agent with TMDB/IMDb, season, episode and release-aware search."
		}
		out = append(out, AgentStatus{Name: provider.Name(), Ready: provider.Ready(), Description: description})
	}
	return out
}

func (s *Service) StartLibraryJob(ctx context.Context, libraryID int64, language string) (Job, error) {
	language = strings.TrimSpace(language)
	if language == "" {
		language = "pt-BR"
	}
	var library string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM libraries WHERE id=?`, libraryID).Scan(&library); err != nil {
		return Job{}, err
	}
	if !s.anyReady() {
		return Job{}, errors.New("no subtitle provider is configured")
	}
	s.mu.Lock()
	if s.running[libraryID] {
		s.mu.Unlock()
		return Job{}, errors.New("a subtitle job is already running for this library")
	}
	s.running[libraryID] = true
	s.mu.Unlock()
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media m JOIN media_metadata mm ON mm.media_id=m.id WHERE m.library_id=? AND m.available=1 AND mm.status='matched'`, libraryID).Scan(&total); err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO subtitle_jobs(library_id,language,status,total) VALUES(?,?,'queued',?)`, libraryID, language, total)
	if err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	id, _ := res.LastInsertId()
	job, err := s.Job(ctx, id)
	if err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	go s.runJob(id, libraryID, language)
	return job, nil
}

func (s *Service) runJob(jobID, libraryID int64, language string) {
	defer s.setRunning(libraryID, false)
	ctx := context.Background()
	_, _ = s.db.ExecContext(ctx, `UPDATE subtitle_jobs SET status='running',started_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,mm.tmdb_id,mm.imdb_id,mm.media_type,mm.season_number,mm.episode_number FROM media m JOIN media_metadata mm ON mm.media_id=m.id WHERE m.library_id=? AND m.available=1 AND mm.status='matched' ORDER BY m.id`, libraryID)
	if err != nil {
		s.finishJob(ctx, jobID, "failed", err.Error())
		return
	}
	queries := []Query{}
	for rows.Next() {
		var q Query
		if err := rows.Scan(&q.MediaID, &q.TMDBID, &q.IMDbID, &q.MediaType, &q.Season, &q.Episode); err != nil {
			_ = rows.Close()
			s.finishJob(ctx, jobID, "failed", err.Error())
			return
		}
		q.Language = language
		queries = append(queries, q)
	}
	_ = rows.Close()

	processed, downloaded, failed := 0, 0, 0
	for _, query := range queries {
		var existing int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subtitles WHERE media_id=? AND lower(language) IN (lower(?),lower(?))`, query.MediaID, language, shortLanguage(language)).Scan(&existing)
		if existing > 0 {
			processed++
			downloaded++
			s.updateProgress(ctx, jobID, processed, downloaded, failed, "subtitle already exists")
			continue
		}
		dl, err := s.download(ctx, query)
		if err != nil {
			failed++
		} else if err := s.save(ctx, query.MediaID, dl); err != nil {
			failed++
		} else {
			downloaded++
		}
		processed++
		s.updateProgress(ctx, jobID, processed, downloaded, failed, fmt.Sprintf("media %d", query.MediaID))
		time.Sleep(150 * time.Millisecond)
	}
	status := "completed"
	if failed > 0 && downloaded == 0 && len(queries) > 0 {
		status = "completed_with_errors"
	}
	s.finishJob(ctx, jobID, status, fmt.Sprintf("%d downloaded, %d failed", downloaded, failed))
}

func (s *Service) download(ctx context.Context, query Query) (Download, error) {
	errs := []string{}
	for _, provider := range s.providers {
		if !provider.Ready() {
			continue
		}
		dl, err := provider.Download(ctx, query)
		if err == nil && len(dl.Data) > 0 {
			if dl.Language == "" {
				dl.Language = query.Language
			}
			return dl, nil
		}
		if err != nil {
			errs = append(errs, provider.Name()+": "+err.Error())
		}
	}
	if len(errs) == 0 {
		return Download{}, errors.New("no ready subtitle provider")
	}
	return Download{}, errors.New(strings.Join(errs, " | "))
}

func (s *Service) save(ctx context.Context, mediaID int64, dl Download) error {
	format := normalizeSubtitleFormat(dl.Format)
	key := fmt.Sprintf("subtitles/%d/%s-%s.%s", mediaID, safeLanguage(dl.Language), dl.Provider, format)
	assetPath, publicURL, err := s.assets.Put(key, bytes.NewReader(dl.Data))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO subtitles(media_id,language,hearing_impaired,format,provider,provider_id,release_name,source_url,asset_path,public_url,score) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, mediaID, dl.Language, dl.HearingImpaired, format, dl.Provider, dl.ProviderID, dl.ReleaseName, dl.SourceURL, assetPath, publicURL, 100)
	return err
}

func (s *Service) ListForMedia(ctx context.Context, mediaID int64) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,media_id,language,hearing_impaired,format,provider,release_name,public_url,created_at FROM subtitles WHERE media_id=? ORDER BY language,provider`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.MediaID, &item.Language, &item.HearingImpaired, &item.Format, &item.Provider, &item.ReleaseName, &item.PublicURL, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) Jobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.library_id,l.name,j.language,j.status,j.total,j.processed,j.downloaded,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM subtitle_jobs j JOIN libraries l ON l.id=j.library_id ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.LibraryID, &job.Library, &job.Language, &job.Status, &job.Total, &job.Processed, &job.Downloaded, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Service) Job(ctx context.Context, id int64) (Job, error) {
	var job Job
	err := s.db.QueryRowContext(ctx, `SELECT j.id,j.library_id,l.name,j.language,j.status,j.total,j.processed,j.downloaded,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM subtitle_jobs j JOIN libraries l ON l.id=j.library_id WHERE j.id=?`, id).
		Scan(&job.ID, &job.LibraryID, &job.Library, &job.Language, &job.Status, &job.Total, &job.Processed, &job.Downloaded, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return job, err
}

func (s *Service) updateProgress(ctx context.Context, jobID int64, processed, downloaded, failed int, message string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE subtitle_jobs SET processed=?,downloaded=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, processed, downloaded, failed, message, jobID)
}

func (s *Service) finishJob(ctx context.Context, jobID int64, status, message string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE subtitle_jobs SET status=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, jobID)
}

func (s *Service) setRunning(libraryID int64, value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value {
		s.running[libraryID] = true
	} else {
		delete(s.running, libraryID)
	}
}

func (s *Service) anyReady() bool {
	for _, provider := range s.providers {
		if provider.Ready() {
			return true
		}
	}
	return false
}

func shortLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if len(language) >= 2 {
		return language[:2]
	}
	return language
}

func safeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	language = strings.NewReplacer("/", "-", "\\", "-", "_", "-").Replace(language)
	if language == "" {
		return "unknown"
	}
	return language
}
