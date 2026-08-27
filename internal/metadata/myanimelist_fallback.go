package metadata

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var stormflixMAL = NewMyAnimeListProvider()
var stormflixAniDB = NewAniDBResolver()
var stormflixAnimeAPI = NewAnimeAPIProvider()
var metadataSpecialPrefixRE = regexp.MustCompile(`(?i)^(?:especiais?|specials?|ovas?|oavs?)\s+`)
var metadataRapturaRE = regexp.MustCompile(`(?i)\braptura\b`)

func isAnimeFallbackLibrary(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "anime" || kind == "mixed" || kind == "anime_series"
}

// RefreshMediaSmart keeps the normal provider strategy first, then safe title
// aliases, AniList and finally AniDB -> MyAnimeList -> AnimeAPI. This order is
// intentionally broader for Brazilian dubbed-anime archives where the folder
// name is often much cleaner than the release filename.
func (s *Service) RefreshMediaSmart(ctx context.Context, mediaID int64) error {
	var item SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path); err != nil {
		return err
	}
	parsed := ParseFilename(item.Path, item.LibraryKind)
	result, primaryErr := s.lookup(ctx, item, parsed)
	if primaryErr == nil {
		return s.saveResult(ctx, mediaID, result)
	}
	if alternate, altErr := s.lookupSafeTitleAlternates(ctx, item, parsed); altErr == nil {
		return s.saveResult(ctx, mediaID, alternate)
	}
	if isAnimeFallbackLibrary(item.LibraryKind) {
		if anilistResult, anilistErr := s.lookupAniListFallback(ctx, item, parsed); anilistErr == nil {
			return s.saveResult(ctx, mediaID, anilistResult)
		}
	}
	fallback, fallbackErr := s.lookupMyAnimeListFallback(ctx, item, parsed)
	if fallbackErr != nil {
		_ = s.saveError(ctx, mediaID, parsed, errors.Join(primaryErr, fallbackErr))
		return errors.Join(primaryErr, fallbackErr)
	}
	return s.saveResult(ctx, mediaID, fallback)
}

// RetryLibraryErrorsWithMyAnimeList is the automatic anime recovery pass run
// after a library metadata job. Despite the historical name it now performs
// the complete recovery chain: safe aliases -> AniList -> AniDB -> MAL ->
// AnimeAPI -> optional TMDB/Fanart enrichment.
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
		if result, altErr := s.lookupSafeTitleAlternates(ctx, item, parsed); altErr == nil {
			_ = s.saveResult(ctx, item.ID, result)
			continue
		}
		if isAnimeFallbackLibrary(item.LibraryKind) {
			if result, anilistErr := s.lookupAniListFallback(ctx, item, parsed); anilistErr == nil {
				_ = s.saveResult(ctx, item.ID, result)
				continue
			}
		}
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

func (s *Service) lookupSafeTitleAlternates(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	alternates := safeMetadataParsedAlternates(parsed)
	var lastErr error = errors.New("no safe metadata alternate")
	for _, candidate := range alternates {
		result, err := s.lookup(ctx, item, candidate)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return Result{}, lastErr
}

func safeMetadataParsedAlternates(parsed ParsedName) []ParsedName {
	out := []ParsedName{}
	seen := map[string]bool{normalizeTitle(parsed.Title): true}
	add := func(title string) {
		title = compactTitle(title)
		key := normalizeTitle(title)
		if title == "" || key == "" || seen[key] {
			return
		}
		seen[key] = true
		candidate := parsed
		candidate.Title = title
		candidate.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		out = append(out, candidate)
	}
	add(metadataRapturaRE.ReplaceAllString(parsed.Title, "Ruptura"))
	add(metadataSpecialPrefixRE.ReplaceAllString(parsed.Title, ""))
	withoutPrefix := metadataSpecialPrefixRE.ReplaceAllString(parsed.Title, "")
	add(metadataRapturaRE.ReplaceAllString(withoutPrefix, "Ruptura"))
	return out
}

func (s *Service) lookupAniListFallback(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if !isAnimeFallbackLibrary(item.LibraryKind) {
		return Result{}, errors.New("AniList fallback skipped: library is not anime-capable")
	}
	s.providerMu.RLock()
	anilist := s.anilist
	s.providerMu.RUnlock()
	if anilist == nil || !anilist.Ready() {
		return Result{}, errors.New("AniList fallback unavailable")
	}

	var lastErr error = errors.New("AniList: no match")
	for _, title := range parsed.SearchTitles() {
		candidate := parsed
		candidate.Title = title
		result, err := anilist.Lookup(ctx, item, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		result.MediaType = "anime"
		result.Season = parsed.Season
		result.Episode = parsed.Episode
		return s.enrichAnimeFallbackResult(ctx, item, parsed, result), nil
	}
	return Result{}, lastErr
}

func (s *Service) lookupMyAnimeListFallback(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if !isAnimeFallbackLibrary(item.LibraryKind) {
		return Result{}, errors.New("MyAnimeList fallback skipped: library is not anime-capable")
	}
	resolved := parsed
	if match, err := stormflixAniDB.Resolve(ctx, parsed.SearchTitles()); err == nil && strings.TrimSpace(match.Title) != "" {
		resolved.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		resolved.Title = match.Title
	}
	result, err := stormflixMAL.Lookup(ctx, item, resolved)
	if err != nil {
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
	return s.enrichAnimeFallbackResult(ctx, item, parsed, result), nil
}

func (s *Service) enrichAnimeFallbackResult(ctx context.Context, item SourceItem, parsed ParsedName, result Result) Result {
	// AnimeAPI is a relation mapper, not a hard dependency. Once AniList/MAL has
	// identified the title, use it best-effort to bridge IDs into TMDB/TVDB/IMDb.
	_ = stormflixAnimeAPI.Enrich(ctx, &result)

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
	return result
}
