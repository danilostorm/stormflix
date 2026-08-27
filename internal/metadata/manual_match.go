package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type CatalogCandidate struct {
	TMDBID        int64  `json:"tmdb_id"`
	MediaType     string `json:"media_type"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          int    `json:"year"`
	Overview      string `json:"overview"`
	PosterURL     string `json:"poster_url"`
}

type manualSearchResponse struct {
	Results []struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		OriginalTitle string `json:"original_title"`
		Name          string `json:"name"`
		OriginalName  string `json:"original_name"`
		ReleaseDate   string `json:"release_date"`
		FirstAirDate  string `json:"first_air_date"`
		Overview      string `json:"overview"`
		PosterPath    string `json:"poster_path"`
	} `json:"results"`
}

func (s *Service) SearchTMDB(ctx context.Context, query, mediaType string, year int) ([]CatalogCandidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []CatalogCandidate{}, nil
	}
	s.providerMu.RLock()
	tmdb := s.tmdb
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return nil, errors.New("TMDB is not configured")
	}
	kinds := []string{"movie", "tv"}
	if mediaType == "movie" || mediaType == "tv" {
		kinds = []string{mediaType}
	}
	out := []CatalogCandidate{}
	seen := map[string]bool{}
	for _, kind := range kinds {
		q := url.Values{}
		q.Set("query", query)
		q.Set("include_adult", "false")
		if tmdb.language != "" {
			q.Set("language", tmdb.language)
		}
		if year > 0 {
			if kind == "movie" {
				q.Set("year", strconv.Itoa(year))
			} else {
				q.Set("first_air_date_year", strconv.Itoa(year))
			}
		}
		var response manualSearchResponse
		if err := tmdb.get(ctx, "https://api.themoviedb.org/3/search/"+kind+"?"+q.Encode(), &response); err != nil {
			return nil, err
		}
		for _, candidate := range response.Results {
			key := kind + ":" + strconv.FormatInt(candidate.ID, 10)
			if seen[key] {
				continue
			}
			seen[key] = true
			title := firstNonEmpty(candidate.Title, candidate.Name)
			original := firstNonEmpty(candidate.OriginalTitle, candidate.OriginalName)
			date := firstNonEmpty(candidate.ReleaseDate, candidate.FirstAirDate)
			poster := ""
			if candidate.PosterPath != "" {
				poster = "https://image.tmdb.org/t/p/w342" + candidate.PosterPath
			}
			out = append(out, CatalogCandidate{TMDBID: candidate.ID, MediaType: kind, Title: title, OriginalTitle: original, Year: yearFromDate(date), Overview: candidate.Overview, PosterURL: poster})
			if len(out) >= 20 {
				return out, nil
			}
		}
	}
	return out, nil
}

func (s *Service) SearchForMedia(ctx context.Context, mediaID int64, query string) ([]CatalogCandidate, error) {
	var path, kind string
	if err := s.db.QueryRowContext(ctx, `SELECT m.path,l.kind FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1`, mediaID).Scan(&path, &kind); err != nil {
		return nil, err
	}
	parsed := ParseFilename(path, kind)
	if strings.TrimSpace(query) == "" {
		query = parsed.Title
	}
	mediaType := ""
	if kind == "movies" {
		mediaType = "movie"
	} else if kind == "series" || kind == "animation_series" || kind == "anime_series" {
		mediaType = "tv"
	}
	return s.SearchTMDB(ctx, query, mediaType, parsed.Year)
}

func (s *Service) ManualMatch(ctx context.Context, mediaID, tmdbID int64, mediaType string, applyCopies bool) (int, error) {
	if tmdbID <= 0 || (mediaType != "movie" && mediaType != "tv") {
		return 0, errors.New("valid TMDB id and media type are required")
	}
	var source SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&source.ID, &source.LibraryID, &source.LibraryKind, &source.Title, &source.Path); err != nil {
		return 0, err
	}
	s.providerMu.RLock()
	tmdb := s.tmdb
	fanart := s.fanart
	theme := s.theme
	cfg := s.cfg
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return 0, errors.New("TMDB is not configured")
	}
	parsed := ParseFilename(source.Path, source.LibraryKind)
	var result Result
	var err error
	if mediaType == "movie" {
		result, err = tmdb.movie(ctx, tmdbID, parsed)
	} else {
		result, err = tmdb.tv(ctx, tmdbID, parsed)
	}
	if err != nil {
		return 0, err
	}
	if source.LibraryKind == "anime" {
		result.MediaType = "anime"
	}
	_ = tmdb.EnrichExperience(ctx, &result)
	_ = fanart.Enrich(ctx, &result)
	if cfg.ThemePreviewEnabled && theme != nil && result.MediaType == "series" {
		if previewURL, previewTitle, previewErr := theme.Lookup(ctx, result.Title, result.Year); previewErr == nil {
			result.ThemePreviewURL = previewURL
			result.ThemePreviewTitle = previewTitle
		}
	}

	targets := []int64{mediaID}
	if applyCopies {
		targets, err = s.logicalCopies(ctx, source, parsed)
		if err != nil {
			return 0, err
		}
	}
	updated := 0
	for _, target := range targets {
		if err := s.saveResult(ctx, target, result); err != nil {
			return updated, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE media_metadata SET manual_match=1,last_error='',updated_at=CURRENT_TIMESTAMP WHERE media_id=?`, target); err != nil {
			return updated, err
		}
		// Subtitles and artwork associated with the wrong title are invalid. The
		// artwork tree was already replaced by saveResult; clean subtitles too.
		_, _ = s.db.ExecContext(ctx, `DELETE FROM subtitles WHERE media_id=?`, target)
		_ = s.assets.RemoveTree(fmt.Sprintf("subtitles/%d", target))
		updated++
	}
	return updated, nil
}

func (s *Service) logicalCopies(ctx context.Context, source SourceItem, parsed ParsedName) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.path,l.kind FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.library_id=? AND m.available=1 ORDER BY m.id`, source.LibraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := normalizeTitle(parsed.Title)
	out := []int64{}
	for rows.Next() {
		var id int64
		var path, kind string
		if err := rows.Scan(&id, &path, &kind); err != nil {
			return nil, err
		}
		candidate := ParseFilename(path, kind)
		if normalizeTitle(candidate.Title) != want {
			continue
		}
		if parsed.Year > 0 && candidate.Year > 0 && parsed.Year != candidate.Year {
			continue
		}
		if parsed.Season > 0 && candidate.Season != parsed.Season {
			continue
		}
		if parsed.Episode > 0 && candidate.Episode != parsed.Episode {
			continue
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []int64{source.ID}, nil
	}
	return out, nil
}

func (s *Service) ResetManualMatch(ctx context.Context, mediaID int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE media_metadata SET manual_match=0,updated_at=CURRENT_TIMESTAMP WHERE media_id=?`, mediaID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return s.RefreshMedia(ctx, mediaID)
}
