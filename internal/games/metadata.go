package games

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxGameArtworkBytes int64 = 25 << 20

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
	active := []string{}
	for _, provider := range providers {
		if provider.Enabled && provider.Configured && (provider.Key == "igdb" || provider.Key == "mobygames" || provider.Key == "steamgriddb") {
			active = append(active, provider.Key)
		}
	}
	if len(active) == 0 {
		return MetadataJob{}, errors.New("configure and enable IGDB, MobyGames or SteamGridDB first")
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
		// Respect the most restrictive currently implemented provider (MobyGames).
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
	var candidate *gameMetadataCandidate
	var errs []string
	if public, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "igdb"); enabled && strings.TrimSpace(public["client_id"]) != "" && strings.TrimSpace(secrets["client_secret"]) != "" {
		value, err := fetchIGDB(ctx, public["client_id"], secrets["client_secret"], game)
		if err != nil {
			errs = append(errs, "IGDB: "+err.Error())
		} else if value != nil {
			candidate = value
		}
	}
	if candidate == nil {
		if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "mobygames"); enabled && strings.TrimSpace(secrets["api_key"]) != "" {
			value, err := fetchMobyGames(ctx, secrets["api_key"], game)
			if err != nil {
				errs = append(errs, "MobyGames: "+err.Error())
			} else if value != nil {
				candidate = value
			}
		}
	}
	if candidate != nil {
		if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "steamgriddb"); enabled && strings.TrimSpace(secrets["api_key"]) != "" {
			id, cover, err := fetchSteamGridDB(ctx, secrets["api_key"], candidate.Title)
			if err != nil {
				errs = append(errs, "SteamGridDB: "+err.Error())
			} else if cover != "" {
				candidate.SteamGridDBID = strconv.FormatInt(id, 10)
				candidate.CoverURL = cover
			}
		}
		return candidate.Provider, candidate, nil
	}
	if len(errs) > 0 {
		return "", nil, errors.New(strings.Join(errs, " · "))
	}
	return "", nil, nil
}

func (s *Service) storeMetadataCandidate(ctx context.Context, game metadataGameRow, candidate gameMetadataCandidate) error {
	var locked bool
	_ = s.db.QueryRowContext(ctx, `SELECT metadata_locked FROM game_metadata WHERE game_id=?`, game.ID).Scan(&locked)
	if locked {
		return nil
	}
	coverPath := ""
	if candidate.CoverURL != "" {
		if path, err := s.downloadGameArtwork(ctx, game, candidate.CoverURL); err == nil {
			coverPath = path
		}
	}
	genres, _ := json.Marshal(candidate.Genres)
	developers, _ := json.Marshal(candidate.Developers)
	publishers, _ := json.Marshal(candidate.Publishers)
	screenshots, _ := json.Marshal(candidate.Screenshots)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO game_metadata(game_id,primary_provider,primary_id,igdb_id,steamgriddb_id,mobygames_id,genres_json,developers_json,publishers_json,screenshots_json,community_rating,last_error,refreshed_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,'',CURRENT_TIMESTAMP)
ON CONFLICT(game_id) DO UPDATE SET
 primary_provider=excluded.primary_provider,primary_id=excluded.primary_id,
 igdb_id=CASE WHEN excluded.igdb_id<>'' THEN excluded.igdb_id ELSE game_metadata.igdb_id END,
 steamgriddb_id=CASE WHEN excluded.steamgriddb_id<>'' THEN excluded.steamgriddb_id ELSE game_metadata.steamgriddb_id END,
 mobygames_id=CASE WHEN excluded.mobygames_id<>'' THEN excluded.mobygames_id ELSE game_metadata.mobygames_id END,
 genres_json=excluded.genres_json,developers_json=excluded.developers_json,publishers_json=excluded.publishers_json,
 screenshots_json=excluded.screenshots_json,community_rating=excluded.community_rating,last_error='',refreshed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
		game.ID, candidate.Provider, candidate.ProviderID,
		providerID(candidate, "igdb"), candidate.SteamGridDBID, providerID(candidate, "mobygames"),
		string(genres), string(developers), string(publishers), string(screenshots), candidate.Rating)
	if err != nil {
		return err
	}
	title := game.Title
	if titleScore(game.Title, candidate.Title) >= .74 && strings.TrimSpace(candidate.Title) != "" {
		title = strings.TrimSpace(candidate.Title)
	}
	if coverPath != "" {
		_, err = tx.ExecContext(ctx, `UPDATE games SET title=?,sort_title=?,overview=?,release_year=?,cover_path=?,metadata_provider=?,metadata_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, strings.ToLower(title), candidate.Overview, candidate.ReleaseYear, coverPath, candidate.Provider, candidate.ProviderID, game.ID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE games SET title=?,sort_title=?,overview=?,release_year=?,metadata_provider=?,metadata_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, strings.ToLower(title), candidate.Overview, candidate.ReleaseYear, candidate.Provider, candidate.ProviderID, game.ID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func providerID(candidate gameMetadataCandidate, provider string) string {
	if candidate.Provider == provider {
		return candidate.ProviderID
	}
	return ""
}

func (s *Service) storeMetadataError(ctx context.Context, gameID int64, cause error) error {
	message := cause.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO game_metadata(game_id,last_error,refreshed_at) VALUES(?,?,CURRENT_TIMESTAMP) ON CONFLICT(game_id) DO UPDATE SET last_error=excluded.last_error,refreshed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, gameID, message)
	return err
}

func (s *Service) finishMetadataJob(jobID int64, processed, matched, failed int, status, message string) {
	if len(message) > 500 {
		message = message[:500]
	}
	_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET status=?,progress=100,processed=?,matched=?,failed=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, processed, matched, failed, message, jobID)
}

func (s *Service) downloadGameArtwork(ctx context.Context, game metadataGameRow, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "StormFlix/0.24 Games")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("artwork HTTP %d", resp.StatusCode)
	}
	ext := artworkExtension(rawURL, resp.Header.Get("Content-Type"))
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir, "game-metadata", game.Platform, game.Hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, old := range []string{"cover.jpg", "cover.png", "cover.webp"} {
		_ = os.Remove(filepath.Join(dir, old))
	}
	path := filepath.Join(dir, "cover"+ext)
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxGameArtworkBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n <= 0 || n > maxGameArtworkBytes {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return "", errors.New("invalid game artwork size")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func artworkExtension(rawURL, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(filepath.Ext(parsed.Path))
		if ext == ".png" || ext == ".webp" || ext == ".jpg" || ext == ".jpeg" {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	return ".jpg"
}

var igdbPlatform = map[string]int{"nes": 18, "snes": 19, "genesis": 29, "gb": 33, "gbc": 22, "gba": 24}

func fetchIGDB(ctx context.Context, clientID, clientSecret string, game metadataGameRow) (*gameMetadataCandidate, error) {
	tokenURL := "https://id.twitch.tv/oauth2/token?client_id=" + url.QueryEscape(clientID) + "&client_secret=" + url.QueryEscape(clientSecret) + "&grant_type=client_credentials"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OAuth HTTP %d", resp.StatusCode)
	}
	var token struct{ AccessToken string `json:"access_token"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return nil, errors.New("OAuth token inválido")
	}
	platformID := igdbPlatform[game.Platform]
	query := fmt.Sprintf(`fields id,name,summary,first_release_date,rating,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,cover.url,screenshots.url; search %q;`, game.Title)
	if platformID > 0 {
		query += fmt.Sprintf(" where platforms = (%d);", platformID)
	}
	query += " limit 10;"
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.igdb.com/v4/games", bytes.NewBufferString(query))
	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "text/plain")
	resp, err = (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("IGDB HTTP %d", resp.StatusCode)
	}
	type named struct{ Name string `json:"name"` }
	type companyLink struct {
		Developer bool  `json:"developer"`
		Publisher bool  `json:"publisher"`
		Company   named `json:"company"`
	}
	type image struct{ URL string `json:"url"` }
	var items []struct {
		ID                int64         `json:"id"`
		Name              string        `json:"name"`
		Summary           string        `json:"summary"`
		FirstReleaseDate  int64         `json:"first_release_date"`
		Rating            float64       `json:"rating"`
		Genres            []named       `json:"genres"`
		InvolvedCompanies []companyLink `json:"involved_companies"`
		Cover             image         `json:"cover"`
		Screenshots       []image       `json:"screenshots"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&items); err != nil {
		return nil, err
	}
	best := -1
	bestScore := 0.0
	for i := range items {
		score := titleScore(game.Title, items[i].Name)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 || bestScore < .62 {
		return nil, nil
	}
	item := items[best]
	out := &gameMetadataCandidate{Provider: "igdb", ProviderID: strconv.FormatInt(item.ID, 10), Title: item.Name, Overview: item.Summary, Rating: item.Rating}
	if item.FirstReleaseDate > 0 {
		out.ReleaseYear = time.Unix(item.FirstReleaseDate, 0).UTC().Year()
	}
	for _, genre := range item.Genres {
		if genre.Name != "" {
			out.Genres = append(out.Genres, genre.Name)
		}
	}
	for _, link := range item.InvolvedCompanies {
		if link.Developer && link.Company.Name != "" {
			out.Developers = append(out.Developers, link.Company.Name)
		}
		if link.Publisher && link.Company.Name != "" {
			out.Publishers = append(out.Publishers, link.Company.Name)
		}
	}
	if item.Cover.URL != "" {
		out.CoverURL = igdbImage(item.Cover.URL, "t_cover_big_2x")
	}
	for _, shot := range item.Screenshots {
		if shot.URL != "" {
			out.Screenshots = append(out.Screenshots, igdbImage(shot.URL, "t_screenshot_big"))
		}
	}
	return out, nil
}

func igdbImage(raw, size string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	return strings.Replace(raw, "t_thumb", size, 1)
}

func fetchMobyGames(ctx context.Context, apiKey string, game metadataGameRow) (*gameMetadataCandidate, error) {
	endpoint := "https://api.mobygames.com/v1/games?format=normal&limit=10&title=" + url.QueryEscape(game.Title) + "&api_key=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "StormFlix/0.24 Games")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Games []struct {
			GameID      int64   `json:"game_id"`
			Title       string  `json:"title"`
			Description string  `json:"description"`
			MobyScore   float64 `json:"moby_score"`
			Genres      []struct {
				Name string `json:"genre_name"`
			} `json:"genres"`
			Platforms []struct {
				Date string `json:"first_release_date"`
				Name string `json:"platform_name"`
			} `json:"platforms"`
			SampleCover struct {
				Image string `json:"image"`
			} `json:"sample_cover"`
			Screenshots []struct {
				Image string `json:"image"`
			} `json:"sample_screenshots"`
		} `json:"games"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&result); err != nil {
		return nil, err
	}
	best := -1
	bestScore := 0.0
	for i := range result.Games {
		score := titleScore(game.Title, result.Games[i].Title)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 || bestScore < .68 {
		return nil, nil
	}
	item := result.Games[best]
	out := &gameMetadataCandidate{Provider: "mobygames", ProviderID: strconv.FormatInt(item.GameID, 10), Title: item.Title, Overview: item.Description, Rating: item.MobyScore, CoverURL: item.SampleCover.Image}
	for _, genre := range item.Genres {
		if genre.Name != "" {
			out.Genres = append(out.Genres, genre.Name)
		}
	}
	for _, platform := range item.Platforms {
		if len(platform.Date) >= 4 {
			if year, err := strconv.Atoi(platform.Date[:4]); err == nil && (out.ReleaseYear == 0 || year < out.ReleaseYear) {
				out.ReleaseYear = year
			}
		}
	}
	for _, shot := range item.Screenshots {
		if shot.Image != "" {
			out.Screenshots = append(out.Screenshots, shot.Image)
		}
	}
	return out, nil
}

func fetchSteamGridDB(ctx context.Context, apiKey, title string) (int64, string, error) {
	client := &http.Client{Timeout: 25 * time.Second}
	endpoint := "https://steamgriddb.com/api/v2/search/autocomplete/" + url.PathEscape(title)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "StormFlix/0.24 Games")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", fmt.Errorf("search HTTP %d", resp.StatusCode)
	}
	var search struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&search); err != nil {
		return 0, "", err
	}
	bestID := int64(0)
	bestScore := 0.0
	for _, item := range search.Data {
		score := titleScore(title, item.Name)
		if score > bestScore {
			bestScore, bestID = score, item.ID
		}
	}
	if bestID == 0 || bestScore < .64 {
		return 0, "", nil
	}
	endpoint = fmt.Sprintf("https://steamgriddb.com/api/v2/grids/game/%d?types=static&nsfw=false&humor=false&limit=50", bestID)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "StormFlix/0.24 Games")
	resp, err = client.Do(req)
	if err != nil {
		return bestID, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bestID, "", fmt.Errorf("grid HTTP %d", resp.StatusCode)
	}
	var grids struct {
		Data []struct {
			URL    string  `json:"url"`
			Score  float64 `json:"score"`
			Width  int     `json:"width"`
			Height int     `json:"height"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&grids); err != nil {
		return bestID, "", err
	}
	sort.SliceStable(grids.Data, func(i, j int) bool {
		portraitI := grids.Data[i].Height > grids.Data[i].Width
		portraitJ := grids.Data[j].Height > grids.Data[j].Width
		if portraitI != portraitJ {
			return portraitI
		}
		return grids.Data[i].Score > grids.Data[j].Score
	})
	for _, grid := range grids.Data {
		if grid.URL != "" && grid.Height > grid.Width {
			return bestID, grid.URL, nil
		}
	}
	return bestID, "", nil
}

func titleScore(a, b string) float64 {
	a, b = normalizeGameTitle(a), normalizeGameTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter, longer := len(a), len(b)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return .75 + .2*float64(shorter)/float64(longer)
	}
	setA, setB := map[string]bool{}, map[string]bool{}
	for _, token := range strings.Fields(a) {
		setA[token] = true
	}
	for _, token := range strings.Fields(b) {
		setB[token] = true
	}
	intersection := 0
	for token := range setA {
		if setB[token] {
			intersection++
		}
	}
	if intersection == 0 {
		return 0
	}
	precision := float64(intersection) / float64(len(setA))
	recall := float64(intersection) / float64(len(setB))
	return 2 * precision * recall / (precision + recall)
}

func normalizeGameTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
		"&", " and ", "_", " ", "-", " ", ":", " ", ".", " ", ",", " ", "'", "", "’", "",
	).Replace(value)
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			out = append(out, r)
		}
	}
	return strings.Join(strings.Fields(string(out)), " ")
}
