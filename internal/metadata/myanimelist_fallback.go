package metadata

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var stormflixMAL = NewMyAnimeListProvider()
var stormflixAniDB = NewAniDBResolver()

// RefreshMediaSmart keeps the regular provider chain first, then retries
// unmatched anime/mixed-animation items against MyAnimeList. This is used by
// the admin reprocess action so old TMDB errors can be recovered immediately.
func (s *Service) RefreshMediaSmart(ctx context.Context, mediaID int64) error {
	var item SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path); err != nil {
		return err
	}
	parsed := ParseFilename(item.Path, item.LibraryKind)
	result, err := s.lookup(ctx, item, parsed)
	if err == nil {
		return s.saveResult(ctx, mediaID, result)
	}
	fallback, fallbackErr := s.lookupMyAnimeListFallback(ctx, item, parsed)
	if fallbackErr != nil {
		_ = s.saveError(ctx, mediaID, parsed, errors.Join(err, fallbackErr))
		return errors.Join(err, fallbackErr)
	}
	return s.saveResult(ctx, mediaID, fallback)
}

// RetryLibraryErrorsWithMyAnimeList is intentionally a second pass. The normal
// scan remains fast and keeps its existing provider priorities; only items that
// actually failed are sent to MyAnimeList/Jikan.
func (s *Service) RetryLibraryErrorsWithMyAnimeList(ctx context.Context, libraryID int64) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path
FROM media m JOIN libraries l ON l.id=m.library_id JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.library_id=? AND m.available=1 AND mm.status='error' ORDER BY m.id`, libraryID)
	if err != nil {
		return
	}
	items := []SourceItem{}
	for rows.Next() {
		var item SourceItem
		if rows.Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path) == nil {
			items = append(items, item)
		}
	}
	_ = rows.Close()
	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		default:
		}
		parsed := ParseFilename(item.Path, item.LibraryKind)
		result, lookupErr := s.lookupMyAnimeListFallback(ctx, item, parsed)
		if lookupErr != nil {
			continue
		}
		_ = s.saveResult(ctx, item.ID, result)
		select {
		case <-time.After(120 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) lookupMyAnimeListFallback(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	kind := strings.ToLower(strings.TrimSpace(item.LibraryKind))
	// Mixed is the preferred type for a library that contains anime films and
	// western animation together. Anime remains supported for existing setups.
	if kind != "anime" && kind != "mixed" {
		return Result{}, errors.New("MyAnimeList fallback skipped: library is not anime/mixed")
	}
	resolved := parsed
	if match, err := stormflixAniDB.Resolve(ctx, parsed.SearchTitles()); err == nil && strings.TrimSpace(match.Title) != "" {
		resolved.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		resolved.Title = match.Title
	}
	result, err := stormflixMAL.Lookup(ctx, item, resolved)
	if err != nil {
		// If AniDB canonicalization was too aggressive, retry the original title.
		if normalizeTitle(resolved.Title) != normalizeTitle(parsed.Title) {
			result, err = stormflixMAL.Lookup(ctx, item, parsed)
		}
		if err != nil {
			return Result{}, err
		}
	}
	result.MediaType = "anime"
	result.Season = parsed.Season
	result.Episode = parsed.Episode

	// Once MAL gives us a canonical title, TMDB sometimes succeeds where the
	// original filename did not. Use that only as enrichment; MAL remains the
	// successful identity and poster source.
	s.providerMu.RLock()
	tmdb := s.tmdb
	fanart := s.fanart
	s.providerMu.RUnlock()
	if tmdb != nil && tmdb.Ready() {
		canonical := parsed
		canonical.Title = result.Title
		canonical.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		if enriched, tmdbErr := tmdb.Lookup(ctx, item, canonical); tmdbErr == nil {
			result.TMDBID = enriched.TMDBID
			result.TVDBID = enriched.TVDBID
			result.IMDbID = enriched.IMDbID
			result.Artwork = append(result.Artwork, enriched.Artwork...)
			_ = tmdb.EnrichExperience(ctx, &enriched)
			if result.Overview == "" {
				result.Overview = enriched.Overview
			}
			result.Cast = enriched.Cast
			result.Directors = enriched.Directors
			if result.TrailerURL == "" {
				result.TrailerURL = enriched.TrailerURL
			}
		}
	}
	if fanart != nil {
		_ = fanart.Enrich(ctx, &result)
	}
	return result, nil
}

// Keep database/sql referenced here so future package-level build tags do not
// accidentally hide the sql import when this file is split by platform.
var _ = sql.ErrNoRows
