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
// The episode refresh itself is queued and observable in Admin -> Fila.
func (s *Service) ManualMatchSeries(ctx context.Context, mediaID, tmdbID int64) (int, int64, error) {
	if tmdbID <= 0 {
		return 0, 0, errors.New("valid TMDB series id is required")
	}
	var source SourceItem
	if err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path,
COALESCE(si.source_root,''),COALESCE(si.series_key,''),COALESCE(si.series_title,''),COALESCE(si.season_number,0),COALESCE(si.episode_number,0),COALESCE(si.absolute_number,0)
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_series_identity si ON si.media_id=m.id
WHERE m.id=? AND m.available=1`, mediaID).Scan(&source.ID, &source.LibraryID, &source.LibraryKind, &source.Title, &source.Path,
		&source.SourceRoot, &source.SeriesKey, &source.SeriesTitle, &source.Season, &source.Episode, &source.Absolute); err != nil {
		return 0, 0, err
	}
	if strings.TrimSpace(source.SeriesKey) == "" {
		return 0, 0, errors.New("this item does not have a scanner-owned series identity yet; run a library scan first")
	}

	s.providerMu.RLock()
	tmdb := s.tmdb
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return 0, 0, errors.New("TMDB is not configured")
	}
	baseParsed := source.Parsed()
	baseParsed.Season = 0
	baseParsed.Episode = 0
	show, err := tmdb.tv(ctx, tmdbID, baseParsed)
	if err != nil {
		return 0, 0, err
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
		return 0, 0, err
	}

	// Rebuild the logical series immediately so all current episodes inherit the
	// approved principal title while keeping scanner-owned season/episode order.
	if err := s.RebuildSeriesIdentities(ctx, source.LibraryID); err != nil {
		return 0, 0, fmt.Errorf("rebuild series identities after manual match: %w", err)
	}

	// Older builds marked every episode as an item-level manual match. Clear that
	// legacy flag: only series_metadata_overrides is protected for this show.
	_, _ = s.db.ExecContext(ctx, `UPDATE media_metadata SET manual_match=0,updated_at=CURRENT_TIMESTAMP
WHERE media_id IN (SELECT media_id FROM media_series_identity WHERE library_id=? AND series_key=?)`, source.LibraryID, source.SeriesKey)

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_series_identity si JOIN media m ON m.id=si.media_id WHERE si.library_id=? AND si.series_key=? AND m.available=1`, source.LibraryID, source.SeriesKey).Scan(&count); err != nil {
		return 0, 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO metadata_jobs(library_id,status,total,message,job_type,series_key,series_title,provider_id) VALUES(?,'queued',?,'aguardando na fila para reorganizar episódios','series_refresh',?,?,?)`, source.LibraryID, count, source.SeriesKey, canonical, tmdbID)
	if err != nil {
		return 0, 0, err
	}
	jobID, _ := res.LastInsertId()
	go s.runQueuedManualSeries(jobID, source.LibraryID, source.SeriesKey, canonical, tmdbID)
	return count, jobID, nil
}

// runQueuedManualSeries waits for another metadata operation on the same
// library to finish, so a principal-series correction never races a full
// library metadata refresh.
func (s *Service) runQueuedManualSeries(jobID, libraryID int64, seriesKey, seriesTitle string, tmdbID int64) {
	ctx := context.Background()
	for {
		s.mu.Lock()
		if !s.running[libraryID] {
			s.running[libraryID] = true
			s.mu.Unlock()
			break
		}
		s.mu.Unlock()
		_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET message='aguardando outro job de metadados da biblioteca terminar',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='queued'`, jobID)
		time.Sleep(500 * time.Millisecond)
	}
	defer s.setRunning(libraryID, false)
	_, _ = s.db.ExecContext(ctx, `UPDATE metadata_jobs SET status='running',started_at=COALESCE(started_at,CURRENT_TIMESTAMP),message=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, "reorganizando "+seriesTitle, jobID)
	s.refreshManualSeries(ctx, jobID, libraryID, seriesKey, tmdbID)
}

func (s *Service) refreshManualSeries(ctx context.Context, jobID, libraryID int64, seriesKey string, tmdbID int64) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.kind,m.title,m.path,
COALESCE(si.source_root,''),COALESCE(si.series_key,''),COALESCE(si.series_title,''),COALESCE(si.season_number,0),COALESCE(si.episode_number,0),COALESCE(si.absolute_number,0)
FROM media_series_identity si JOIN media m ON m.id=si.media_id JOIN libraries l ON l.id=m.library_id
WHERE si.library_id=? AND si.series_key=? AND m.available=1 ORDER BY si.season_number,si.episode_number,m.path`, libraryID, seriesKey)
	if err != nil {
		s.finishJob(ctx, jobID, "failed", err.Error())
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
		s.finishJob(ctx, jobID, "failed", "TMDB is not configured")
		return
	}
	processed, matched, failed := 0, 0, 0
	for _, item := range items {
		if err := s.gate.Wait(ctx, "series_metadata", func(paused bool) {
			message := "reorganizando " + firstNonEmpty(item.SeriesTitle, item.Title)
			if paused {
				message = "Pausado para priorizar reprodução ativa"
			}
			s.updateProgress(ctx, jobID, processed, matched, failed, message)
		}); err != nil {
			s.finishJob(ctx, jobID, "failed", err.Error())
			return
		}
		parsed := item.Parsed()
		label := fmt.Sprintf("T%02d E%02d", maxInt(parsed.Season, 1), maxInt(parsed.Episode, processed+1))
		result, lookupErr := tmdb.tv(ctx, tmdbID, parsed)
		if lookupErr != nil {
			failed++
			_ = s.saveError(ctx, item.ID, parsed, fmt.Errorf("manual series match TMDB %d: %w", tmdbID, lookupErr))
		} else {
			if item.LibraryKind == "anime" || item.LibraryKind == "anime_series" {
				result.MediaType = "anime"
			}
			_ = tmdb.EnrichExperience(ctx, &result)
			if fanart != nil {
				_ = fanart.Enrich(ctx, &result)
			}
			if err := s.saveResult(ctx, item.ID, result); err != nil {
				failed++
				_ = s.saveError(ctx, item.ID, parsed, err)
			} else {
				matched++
			}
		}
		processed++
		s.updateProgress(ctx, jobID, processed, matched, failed, label)
		select {
		case <-ctx.Done():
			s.finishJob(ctx, jobID, "failed", ctx.Err().Error())
			return
		case <-time.After(80 * time.Millisecond):
		}
	}
	status := "completed"
	if failed > 0 {
		status = "completed_with_errors"
	}
	s.finishJob(ctx, jobID, status, fmt.Sprintf("obra principal aplicada · %d episódio(s) atualizados · %d erro(s)", matched, failed))
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
