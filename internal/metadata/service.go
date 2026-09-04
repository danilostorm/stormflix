package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/workload"
)

type Service struct {
	db         *sql.DB
	assets     *assets.Store
	providerMu sync.RWMutex
	cfg        config.Config
	tmdb       *TMDBProvider
	tvdb       *TVDBProvider
	anilist    *AniListProvider
	hama       *HAMAMapper
	fanart     *FanartProvider
	theme      *ThemeProvider
	mu         sync.Mutex
	running    map[int64]bool
	gate       *workload.Gate
}

type Job struct {
	ID         int64   `json:"id"`
	LibraryID  int64   `json:"library_id"`
	Library    string  `json:"library"`
	Status     string  `json:"status"`
	Total      int     `json:"total"`
	Processed  int     `json:"processed"`
	Matched    int     `json:"matched"`
	Failed     int     `json:"failed"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	UpdatedAt  string  `json:"updated_at"`
}

func NewService(db *sql.DB, cfg config.Config, store *assets.Store) *Service {
	s := &Service{db: db, assets: store, running: map[int64]bool{}, gate: workload.For(db)}
	s.Configure(cfg)
	return s
}

func (s *Service) Configure(cfg config.Config) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.cfg = cfg
	s.tmdb = NewTMDBProvider(cfg.TMDBToken, cfg.TMDBAPIKey, cfg.MetadataLanguage)
	s.tvdb = NewTVDBProvider(cfg.TVDBAPIKey, cfg.TVDBPIN, cfg.MetadataLanguage)
	s.anilist = NewAniListProvider()
	if s.hama == nil {
		s.hama = NewHAMAMapper()
	}
	s.fanart = NewFanartProvider(cfg.FanartAPIKey, cfg.FanartClientKey)
	s.theme = NewThemeProvider(cfg.ThemePreviewCountry)
}

func (s *Service) RecoverInterruptedJobs() {
	_, _ = s.db.Exec(`UPDATE metadata_jobs SET status='failed',message='server restarted while job was running',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running')`)
}

func (s *Service) Agents() []AgentStatus {
	s.providerMu.RLock()
	defer s.providerMu.RUnlock()
	return []AgentStatus{
		{Name: "TMDB", Enabled: true, Ready: s.tmdb.Ready(), Description: "Filmes e séries: títulos, sinopses, classificação indicativa, lançamento, elenco, direção, trailer, gêneros, IDs, posters, backdrops, logos e episódios."},
		{Name: "TheTVDB", Enabled: true, Ready: s.tvdb.Ready(), Description: "Fallback para séries e desenhos, com temporadas, episódios e suporte às ordens do TheTVDB v4."},
		{Name: "AniList", Enabled: true, Ready: s.anilist.Ready(), Description: "Agente principal para anime: títulos, sinopses, capas, banners, gêneros e IDs AniList/MAL."},
		{Name: "HAMA / Anime-Lists", Enabled: true, Ready: s.hama.Ready(), Description: "Ponte de IDs inspirada no HAMA: AniDB/AniList/MAL ↔ TVDB/TMDB usando os mapeamentos comunitários Anime-Lists."},
		{Name: "Fanart.tv", Enabled: true, Ready: s.fanart.Ready(), Description: "Artwork extra: logos, clearart, fanart, posters e backgrounds de alta qualidade."},
		{Name: "Trilha Preview", Enabled: s.cfg.ThemePreviewEnabled, Ready: s.cfg.ThemePreviewEnabled, Description: "Prévia curta opcional da abertura de séries; nunca baixa ou toca a faixa completa."},
	}
}

func (s *Service) StartLibraryJob(ctx context.Context, libraryID int64, refresh bool) (Job, error) {
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM libraries WHERE id=?`, libraryID).Scan(&name); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	if s.running[libraryID] {
		s.mu.Unlock()
		return Job{}, errors.New("a metadata scan is already running for this library")
	}
	s.running[libraryID] = true
	s.mu.Unlock()

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE library_id=? AND available=1`, libraryID).Scan(&total); err != nil {
		s.setRunning(libraryID, false)
		return Job{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO metadata_jobs(library_id,status,total,message) VALUES(?, 'queued', ?, ?)`, libraryID, total, map[bool]string{true: "refresh", false: "scan"}[refresh])
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
	go s.runJob(id, libraryID, refresh)
	return job, nil
}

func (s *Service) runJob(jobID, libraryID int64, refresh bool) {
	defer s.setRunning(libraryID, false)
	ctx := context.Background()
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status='running',started_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, jobID)

	if refresh {
		if repaired, err := s.PrepareAnimationLibraryRepair(ctx, libraryID); err != nil {
			s.finishJob(ctx, jobID, "failed", "falha ao reparar metadados de desenhos: "+err.Error())
			return
		} else if repaired > 0 {
			_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, fmt.Sprintf("reparando %d itens de desenhos antes do refresh", repaired), jobID)
		}
	}

	items, err := s.libraryItems(ctx, libraryID)
	if err != nil {
		s.finishJob(ctx, jobID, "failed", err.Error())
		return
	}
	processed, matched, failed := 0, 0, 0
	for _, item := range items {
		if err := s.gate.Wait(ctx, "video_metadata", func(paused bool) {
			message := firstNonEmpty(item.SeriesTitle, item.Title)
			if paused {
				message = "Pausado para priorizar reprodução ativa"
			}
			s.updateProgress(ctx, jobID, processed, matched, failed, message)
		}); err != nil {
			s.finishJob(ctx, jobID, "failed", err.Error())
			return
		}
		var currentStatus string
		var manualMatch bool
		metaErr := s.db.QueryRowContext(ctx, `SELECT status,COALESCE(manual_match,0) FROM media_metadata WHERE media_id=?`, item.ID).Scan(&currentStatus, &manualMatch)
		if metaErr == nil && manualMatch {
			processed++
			matched++
			s.updateProgress(ctx, jobID, processed, matched, failed, "correspondência manual protegida")
			continue
		}
		if !refresh && metaErr == nil && currentStatus == "matched" {
			processed++
			matched++
			s.updateProgress(ctx, jobID, processed, matched, failed, "already matched")
			continue
		}
		parsed := item.Parsed()
		result, lookupErr := s.lookup(ctx, item, parsed)
		if lookupErr != nil {
			failed++
			_ = s.saveError(ctx, item.ID, parsed, lookupErr)
		} else if err := s.saveResult(ctx, item.ID, result); err != nil {
			failed++
			_ = s.saveError(ctx, item.ID, parsed, err)
		} else {
			matched++
		}
		processed++
		progressName := firstNonEmpty(item.SeriesTitle, item.Title)
		s.updateProgress(ctx, jobID, processed, matched, failed, progressName)
		select {
		case <-time.After(120 * time.Millisecond):
		case <-ctx.Done():
			s.finishJob(ctx, jobID, "failed", ctx.Err().Error())
			return
		}
	}
	status := "completed"
	message := fmt.Sprintf("%d matched, %d failed", matched, failed)
	if failed > 0 && matched == 0 && len(items) > 0 {
		status = "completed_with_errors"
	}
	s.finishJob(ctx, jobID, status, message)
}

func (s *Service) lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if strings.TrimSpace(parsed.Title) == "" {
		return Result{}, errors.New("could not derive a title from scanner/filename")
	}
	originalKind := item.LibraryKind
	providerItem := item
	if strings.EqualFold(providerItem.LibraryKind, "animation_series") {
		providerItem.LibraryKind = "series"
	}
	s.providerMu.RLock()
	cfg := s.cfg
	tmdb := s.tmdb
	tvdb := s.tvdb
	anilist := s.anilist
	hama := s.hama
	fanart := s.fanart
	theme := s.theme
	s.providerMu.RUnlock()

	var result Result
	var err error
	if originalKind == "anime" {
		result, err = anilist.Lookup(ctx, item, parsed)
		if err == nil {
			result.ContentRatingAge = -1
			_ = hama.Enrich(ctx, &result)
			if tmdb.Ready() {
				if tmdbResult, tmdbErr := tmdb.Lookup(ctx, item, parsed); tmdbErr == nil {
					result.TMDBID = firstPositiveInt64(result.TMDBID, tmdbResult.TMDBID)
					result.TVDBID = firstPositiveInt64(result.TVDBID, tmdbResult.TVDBID)
					result.IMDbID = firstNonEmpty(result.IMDbID, tmdbResult.IMDbID)
					result.Artwork = append(result.Artwork, tmdbResult.Artwork...)
					_ = tmdb.EnrichExperience(ctx, &tmdbResult)
					result.OriginalTitle = firstNonEmpty(result.OriginalTitle, tmdbResult.OriginalTitle)
					result.Tagline = firstNonEmpty(result.Tagline, tmdbResult.Tagline)
					result.ReleaseDate = firstNonEmpty(result.ReleaseDate, tmdbResult.ReleaseDate)
					result.ContentRating = firstNonEmpty(result.ContentRating, tmdbResult.ContentRating)
					if result.ContentRatingAge < 0 {
						result.ContentRatingAge = tmdbResult.ContentRatingAge
					}
					result.Cast = tmdbResult.Cast
					result.Directors = tmdbResult.Directors
					result.TrailerURL = firstNonEmpty(result.TrailerURL, tmdbResult.TrailerURL)
				}
			}
		} else if tmdb.Ready() {
			result, err = tmdb.Lookup(ctx, item, parsed)
			if err == nil {
				result.MediaType = "anime"
			}
		}
	} else {
		var tmdbErr, tvdbErr error
		if tmdb.Ready() {
			result, tmdbErr = tmdb.Lookup(ctx, providerItem, parsed)
		}
		if (tmdbErr != nil || !tmdb.Ready()) && tvdb.Ready() && tvdb.Supports(originalKind) {
			result, tvdbErr = tvdb.Lookup(ctx, item, parsed)
			if tvdbErr == nil {
				tmdbErr = nil
			}
		}
		if tmdbErr != nil {
			if tvdbErr != nil {
				return Result{}, errors.Join(tmdbErr, tvdbErr)
			}
			return Result{}, tmdbErr
		}
		if !tmdb.Ready() && !tvdb.Ready() {
			return Result{}, errors.New("TMDB/TheTVDB are not configured")
		}
		if result.Title == "" {
			return Result{}, errors.New("series metadata providers returned no match")
		}
	}
	if err != nil {
		return Result{}, err
	}
	if result.TMDBID > 0 && tmdb.Ready() {
		_ = tmdb.EnrichExperience(ctx, &result)
	}
	_ = fanart.Enrich(ctx, &result)
	if cfg.ThemePreviewEnabled && theme != nil && result.MediaType == "series" {
		if previewURL, previewTitle, previewErr := theme.Lookup(ctx, result.Title, result.Year); previewErr == nil {
			result.ThemePreviewURL = previewURL
			result.ThemePreviewTitle = previewTitle
		}
	}
	return result, nil
}

func (s *Service) libraryItems(ctx context.Context, libraryID int64) ([]SourceItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path,
COALESCE(si.source_root,''),COALESCE(si.series_key,''),COALESCE(si.series_title,''),COALESCE(si.season_number,0),COALESCE(si.episode_number,0),COALESCE(si.absolute_number,0)
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_series_identity si ON si.media_id=m.id
WHERE m.library_id=? AND m.available=1 ORDER BY m.id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SourceItem{}
	for rows.Next() {
		var item SourceItem
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path,
			&item.SourceRoot, &item.SeriesKey, &item.SeriesTitle, &item.Season, &item.Episode, &item.Absolute); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) saveResult(ctx context.Context, mediaID int64, result Result) error {
	genres, _ := json.Marshal(result.Genres)
	cast, _ := json.Marshal(result.Cast)
	directors, _ := json.Marshal(result.Directors)
	if strings.TrimSpace(result.ContentRating) == "" {
		result.ContentRatingAge = -1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO media_metadata(media_id,media_type,year,season_number,episode_number,overview,genres_json,rating,runtime_minutes,provider,provider_id,tmdb_id,tvdb_id,imdb_id,anilist_id,mal_id,original_title,tagline,cast_json,directors_json,trailer_url,theme_preview_url,theme_preview_title,release_date,content_rating,content_rating_age,manual_match,status,last_error,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,'matched','',CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET
 media_type=excluded.media_type,year=excluded.year,season_number=excluded.season_number,episode_number=excluded.episode_number,
 overview=excluded.overview,genres_json=excluded.genres_json,rating=excluded.rating,runtime_minutes=excluded.runtime_minutes,
 provider=excluded.provider,provider_id=excluded.provider_id,tmdb_id=excluded.tmdb_id,tvdb_id=excluded.tvdb_id,imdb_id=excluded.imdb_id,
 anilist_id=excluded.anilist_id,mal_id=excluded.mal_id,original_title=excluded.original_title,tagline=excluded.tagline,
 cast_json=excluded.cast_json,directors_json=excluded.directors_json,trailer_url=excluded.trailer_url,
 theme_preview_url=excluded.theme_preview_url,theme_preview_title=excluded.theme_preview_title,
 release_date=excluded.release_date,content_rating=excluded.content_rating,content_rating_age=excluded.content_rating_age,
 manual_match=0,status='matched',last_error='',updated_at=CURRENT_TIMESTAMP`,
		mediaID, result.MediaType, result.Year, result.Season, result.Episode, result.Overview, string(genres), result.Rating, result.RuntimeMinutes,
		result.Provider, result.ProviderID, result.TMDBID, result.TVDBID, result.IMDbID, result.AniListID, result.MALID,
		result.OriginalTitle, result.Tagline, string(cast), string(directors), result.TrailerURL, result.ThemePreviewURL, result.ThemePreviewTitle,
		result.ReleaseDate, result.ContentRating, result.ContentRatingAge)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Title) != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE media SET title=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, result.Title, mediaID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_artwork WHERE media_id=?`, mediaID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := s.assets.RemoveTree(fmt.Sprintf("artwork/%d", mediaID)); err != nil {
		return fmt.Errorf("clean old artwork: %w", err)
	}
	return s.saveArtwork(ctx, mediaID, result.Artwork)
}

func (s *Service) saveArtwork(ctx context.Context, mediaID int64, artwork []Artwork) error {
	if len(artwork) == 0 {
		return nil
	}
	sort.SliceStable(artwork, func(i, j int) bool { return artwork[i].Score > artwork[j].Score })
	selectedKinds := map[string]bool{}
	for _, art := range artwork {
		if art.URL == "" || art.Kind == "" {
			continue
		}
		selected := !selectedKinds[art.Kind]
		assetPath, publicURL := "", ""
		if selected {
			selectedKinds[art.Kind] = true
			key := fmt.Sprintf("artwork/%d/%s", mediaID, art.Kind)
			if path, u, err := s.assets.PutURL(ctx, key, art.URL); err == nil {
				assetPath, publicURL = path, u
			} else {
				publicURL = art.URL
			}
		}
		_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO media_artwork(media_id,kind,provider,source_url,asset_path,public_url,language,score,selected,updated_at) VALUES(?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
			mediaID, art.Kind, art.Provider, art.URL, assetPath, publicURL, art.Language, art.Score, selected)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) saveError(ctx context.Context, mediaID int64, parsed ParsedName, cause error) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_metadata(media_id,media_type,year,season_number,episode_number,status,last_error,updated_at)
VALUES(?,?,?,?,?,'error',?,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET media_type=excluded.media_type,year=excluded.year,season_number=excluded.season_number,episode_number=excluded.episode_number,status='error',last_error=excluded.last_error,updated_at=CURRENT_TIMESTAMP`,
		mediaID, "", parsed.Year, parsed.Season, parsed.Episode, cause.Error())
	return err
}

func (s *Service) updateProgress(ctx context.Context, jobID int64, processed, matched, failed int, message string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET processed=?,matched=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, processed, matched, failed, message, jobID)
}

func (s *Service) finishJob(ctx context.Context, jobID int64, status, message string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, message, jobID)
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

func (s *Service) Job(ctx context.Context, id int64) (Job, error) {
	var job Job
	err := s.db.QueryRowContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.total,j.processed,j.matched,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM metadata_jobs j JOIN libraries l ON l.id=j.library_id WHERE j.id=?`, id).
		Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return job, err
}

func (s *Service) Jobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `SELECT j.id,j.library_id,l.name,j.status,j.total,j.processed,j.matched,j.failed,j.message,j.created_at,j.started_at,j.finished_at,j.updated_at FROM metadata_jobs j JOIN libraries l ON l.id=j.library_id ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.LibraryID, &job.Library, &job.Status, &job.Total, &job.Processed, &job.Matched, &job.Failed, &job.Message, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Service) RefreshMedia(ctx context.Context, mediaID int64) error {
	var item SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path); err != nil {
		return err
	}
	loadSourceItemIdentity(ctx, s.db, &item)
	var manual bool
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(manual_match,0) FROM media_metadata WHERE media_id=?`, mediaID).Scan(&manual)
	if manual {
		return errors.New("esta mídia possui correspondência manual protegida; use Catálogo para alterar a correspondência")
	}
	parsed := item.Parsed()
	result, err := s.lookup(ctx, item, parsed)
	if err != nil {
		_ = s.saveError(ctx, mediaID, parsed, err)
		return err
	}
	return s.saveResult(ctx, mediaID, result)
}

func (s *Service) StatusCounts(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status,COUNT(*) FROM media_metadata GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{"matched": 0, "error": 0, "pending": 0}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	var total, described int64
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE available=1`).Scan(&total)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_metadata`).Scan(&described)
	if total > described {
		out["pending"] += total - described
	}
	return out, nil
}

func (s *Service) Artwork(ctx context.Context, mediaID int64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,provider,source_url,public_url,language,score,selected FROM media_artwork WHERE media_id=? ORDER BY kind,selected DESC,score DESC`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var kind, provider, sourceURL, publicURL, language string
		var score float64
		var selected bool
		if err := rows.Scan(&id, &kind, &provider, &sourceURL, &publicURL, &language, &score, &selected); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "kind": kind, "provider": provider, "source_url": sourceURL, "public_url": publicURL, "language": language, "score": score, "selected": selected})
	}
	return out, rows.Err()
}

func (s *Service) SelectArtwork(ctx context.Context, mediaID, artworkID int64) error {
	var kind string
	if err := s.db.QueryRowContext(ctx, `SELECT kind FROM media_artwork WHERE id=? AND media_id=?`, artworkID, mediaID).Scan(&kind); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE media_artwork SET selected=0,updated_at=CURRENT_TIMESTAMP WHERE media_id=? AND kind=?`, mediaID, kind); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_artwork SET selected=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, artworkID); err != nil {
		return err
	}
	return tx.Commit()
}

func providerIDInt(value string) int64 {
	v, _ := strconv.ParseInt(value, 10, 64)
	return v
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
