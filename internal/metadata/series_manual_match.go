package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ManualMatchSeries stores one provider decision for the whole logical show.
// The manual decision belongs to the principal series, never to individual
// episodes. Scanner-owned season/episode numbering remains authoritative.
func (s *Service) ManualMatchSeries(ctx context.Context, mediaID, tmdbID int64) (int, error) {
	if tmdbID <= 0 {
		return 0, errors.New("valid TMDB series id is required")
	}
	var source SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path,
COALESCE(si.source_root,''),COALESCE(si.series_key,''),COALESCE(si.series_title,''),COALESCE(si.season_number,0),COALESCE(si.episode_number,0),COALESCE(si.absolute_number,0)
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_series_identity si ON si.media_id=m.id
WHERE m.id=? AND m.available=1`, mediaID).Scan(&source.ID, &source.LibraryID, &source.LibraryKind, &source.Title, &source.Path,
		&source.SourceRoot, &source.SeriesKey, &source.SeriesTitle, &source.Season, &source.Episode, &source.Absolute); err != nil {
		return 0, err
	}
	if strings.TrimSpace(source.SeriesKey) == "" {
		return 0, errors.New("this item does not have a scanner-owned series identity yet; run a library scan first")
	}

	s.providerMu.RLock()
	tmdb := s.tmdb
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return 0, errors.New("TMDB is not configured")
	}
	baseParsed := source.Parsed()
	baseParsed.Season = 0
	baseParsed.Episode = 0
	show, err := tmdb.tv(ctx, tmdbID, baseParsed)
	if err != nil {
		return 0, err
	}
	canonical := strings.TrimSpace(show.Title)
	if canonical == "" {
		canonical = firstNonEmpty(source.SeriesTitle, baseParsed.Title)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO series_metadata_overrides(library_id,series_key,provider,provider_id,media_type,title,manual,updated_at)
VALUES(?,?,'tmdb',?,'tv',?,1,CURRENT_TIMESTAMP)
ON CONFLICT(library_id,series_key) DO UPDATE SET provider='tmdb',provider_id=excluded.provider_id,media_type='tv',title=excluded.title,manual=1,updated_at=CURRENT_TIMESTAMP`,
		source.LibraryID, source.SeriesKey, tmdbID, canonical)
	if err != nil {
		return 0, err
	}

	// Rebuild the logical series immediately so all current episodes inherit the
	// approved principal title while keeping scanner-owned season/episode order.
	if err := s.RebuildSeriesIdentities(ctx, source.LibraryID); err != nil {
		return 0, fmt.Errorf("rebuild series identities after manual match: %w", err)
	}

	// Older builds marked every episode as an item-level manual match. Clear that
	// legacy flag: only series_metadata_overrides is protected for this show.
	_, _ = s.db.ExecContext(ctx, `UPDATE media_metadata SET manual_match=0,updated_at=CURRENT_TIMESTAMP
WHERE media_id IN (SELECT media_id FROM media_series_identity WHERE library_id=? AND series_key=?)`, source.LibraryID, source.SeriesKey)

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_series_identity si JOIN media m ON m.id=si.media_id WHERE si.library_id=? AND si.series_key=? AND m.available=1`, source.LibraryID, source.SeriesKey).Scan(&count); err != nil {
		return 0, err
	}
	go s.refreshManualSeries(context.Background(), source.LibraryID, source.SeriesKey, tmdbID)
	return count, nil
}

func (s *Service) refreshManualSeries(ctx context.Context, libraryID int64, seriesKey string, tmdbID int64) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path,
COALESCE(si.source_root,''),COALESCE(si.series_key,''),COALESCE(si.series_title,''),COALESCE(si.season_number,0),COALESCE(si.episode_number,0),COALESCE(si.absolute_number,0)
FROM media_series_identity si JOIN media m ON m.id=si.media_id JOIN libraries l ON l.id=m.library_id
WHERE si.library_id=? AND si.series_key=? AND m.available=1 ORDER BY si.season_number,si.episode_number,m.path`, libraryID, seriesKey)
	if err != nil {
		return
	}
	items := []SourceItem{}
	for rows.Next() {
		var item SourceItem
		if rows.Scan(&item.ID, &item.LibraryID, &item.LibraryKind, &item.Title, &item.Path,
			&item.SourceRoot, &item.SeriesKey, &item.SeriesTitle, &item.Season, &item.Episode, &item.Absolute) == nil {
			items = append(items, item)
		}
	}
	_ = rows.Close()

	s.providerMu.RLock()
	tmdb := s.tmdb
	fanart := s.fanart
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return
	}
	for _, item := range items {
		parsed := item.Parsed()
		result, lookupErr := tmdb.tv(ctx, tmdbID, parsed)
		if lookupErr != nil {
			_ = s.saveError(ctx, item.ID, parsed, fmt.Errorf("manual series match TMDB %d: %w", tmdbID, lookupErr))
			continue
		}
		if item.LibraryKind == "anime" || item.LibraryKind == "anime_series" {
			result.MediaType = "anime"
		}
		_ = tmdb.EnrichExperience(ctx, &result)
		if fanart != nil {
			_ = fanart.Enrich(ctx, &result)
		}
		if err := s.saveResult(ctx, item.ID, result); err != nil {
			_ = s.saveError(ctx, item.ID, parsed, err)
			continue
		}
		// Do not set media_metadata.manual_match here. Episode metadata is derived
		// automatically from the one protected series-level provider decision.
		select {
		case <-ctx.Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
	}
}

func (s *Service) ResetSeriesManualMatch(ctx context.Context, mediaID int64) (int, error) {
	var libraryID int64
	var seriesKey string
	if err := s.db.QueryRowContext(ctx, `SELECT si.library_id,si.series_key FROM media_series_identity si WHERE si.media_id=?`, mediaID).Scan(&libraryID, &seriesKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, sql.ErrNoRows
		}
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM series_metadata_overrides WHERE library_id=? AND series_key=?`, libraryID, seriesKey); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE media_metadata SET manual_match=0,updated_at=CURRENT_TIMESTAMP WHERE media_id IN (SELECT media_id FROM media_series_identity WHERE library_id=? AND series_key=?)`, libraryID, seriesKey)
	if err != nil {
		return 0, err
	}
	_ = s.RebuildSeriesIdentities(ctx, libraryID)
	count64, _ := res.RowsAffected()
	return int(count64), nil
}
