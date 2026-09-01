package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/markerdetect"
)

var creditAnalyzerOnce sync.Once
var creditAnalyzerKick = make(chan struct{}, 1)

type creditEpisode struct {
	ID               int64
	Path             string
	ModifiedUnix     int64
	DurationSeconds  float64
	EpisodeNumber    int
	AnalysisModified int64
	ExistingSource   string
	Fingerprint      markerdetect.CreditFingerprint
	ExtractErr       error
}

func (s *server) startCreditAnalyzer(ctx context.Context) {
	creditAnalyzerOnce.Do(func() {
		_, _ = s.db.ExecContext(context.Background(), `
UPDATE credit_analysis_jobs
SET status='error',progress=100,message='Interrompido por reinício do servidor; a temporada será analisada novamente.',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
WHERE status='running'`)
		go func() {
			// Intro analysis starts first. Credits wait a little longer so upgrades do
			// not launch two remote tail/head reads at the same instant.
			select {
			case <-ctx.Done():
				return
			case <-time.After(35 * time.Second):
			}
			for {
				if s.markerAnalysisPlaybackBusy(ctx) || s.introAnalysisRunning(ctx) {
					if !waitCreditAnalyzer(ctx, 30*time.Second) {
						return
					}
					continue
				}
				worked := s.analyzeOneCreditSeason(ctx)
				if worked {
					if !waitCreditAnalyzer(ctx, 8*time.Second) {
						return
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-creditAnalyzerKick:
				case <-time.After(60 * time.Second):
				}
			}
		}()
	})
}

func waitCreditAnalyzer(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-creditAnalyzerKick:
		return true
	case <-time.After(delay):
		return true
	}
}

func (s *server) kickCreditAnalyzer() {
	select {
	case creditAnalyzerKick <- struct{}{}:
	default:
	}
}

func (s *server) introAnalysisRunning(ctx context.Context) bool {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM marker_analysis_jobs WHERE status='running'`).Scan(&count)
	return err == nil && count > 0
}

func (s *server) nextCreditSeason(ctx context.Context) (markerSeason, error) {
	var season markerSeason
	err := s.db.QueryRowContext(ctx, `
SELECT si.library_id,l.name,si.series_key,COALESCE(si.series_title,''),si.season_number,
       (SELECT COUNT(*)
          FROM media sm
          JOIN media_series_identity ssi ON ssi.media_id=sm.id
         WHERE sm.available=1
           AND ssi.library_id=si.library_id
           AND ssi.series_key=si.series_key
           AND ssi.season_number=si.season_number
           AND ssi.episode_number>0) AS season_size
FROM media m
JOIN libraries l ON l.id=m.library_id
JOIN media_series_identity si ON si.media_id=m.id
JOIN media_technical mt ON mt.media_id=m.id
LEFT JOIN media_credit_analysis ca ON ca.media_id=m.id
WHERE m.available=1
  AND si.series_key<>''
  AND si.episode_number>0
  AND mt.status='ok'
  AND mt.source_modified_unix=m.modified_unix
  AND mt.duration_seconds>=120
  AND (
       ca.media_id IS NULL
       OR ca.source_modified_unix<>m.modified_unix
       OR ca.season_size<>(SELECT COUNT(*)
            FROM media sm
            JOIN media_series_identity ssi ON ssi.media_id=sm.id
           WHERE sm.available=1
             AND ssi.library_id=si.library_id
             AND ssi.series_key=si.series_key
             AND ssi.season_number=si.season_number
             AND ssi.episode_number>0)
       OR ca.credit_status='pending'
       OR (ca.credit_status='error' AND (ca.analyzed_at IS NULL OR ca.analyzed_at<=datetime('now','-6 hours')))
  )
ORDER BY CASE WHEN ca.media_id IS NULL THEN 0 WHEN ca.credit_status='pending' THEN 1 ELSE 2 END,m.id
LIMIT 1`).Scan(&season.LibraryID, &season.Library, &season.SeriesKey, &season.SeriesTitle, &season.Season, &season.Size)
	if err != nil {
		return markerSeason{}, err
	}
	if season.Size < 2 {
		return markerSeason{}, sql.ErrNoRows
	}
	if strings.TrimSpace(season.SeriesTitle) == "" {
		season.SeriesTitle = season.SeriesKey
	}
	return season, nil
}

func (s *server) creditSeasonEpisodes(ctx context.Context, season markerSeason) ([]creditEpisode, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id,m.path,m.modified_unix,mt.duration_seconds,si.episode_number,
       COALESCE(ca.source_modified_unix,0),COALESCE(mm.source,'')
FROM media m
JOIN media_series_identity si ON si.media_id=m.id
JOIN media_technical mt ON mt.media_id=m.id
LEFT JOIN media_credit_analysis ca ON ca.media_id=m.id
LEFT JOIN media_markers mm ON mm.media_id=m.id AND mm.kind='credits'
WHERE m.available=1
  AND si.library_id=? AND si.series_key=? AND si.season_number=? AND si.episode_number>0
  AND mt.status='ok' AND mt.source_modified_unix=m.modified_unix AND mt.duration_seconds>=120
ORDER BY CASE
           WHEN ca.media_id IS NULL OR ca.source_modified_unix<>m.modified_unix OR ca.season_size<>? OR ca.credit_status='pending' THEN 0
           ELSE 1
         END,
         si.episode_number,m.id
LIMIT 12`, season.LibraryID, season.SeriesKey, season.Season, season.Size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creditEpisode{}
	for rows.Next() {
		var episode creditEpisode
		if err := rows.Scan(&episode.ID, &episode.Path, &episode.ModifiedUnix, &episode.DurationSeconds, &episode.EpisodeNumber, &episode.AnalysisModified, &episode.ExistingSource); err != nil {
			return nil, err
		}
		out = append(out, episode)
	}
	return out, rows.Err()
}

func (s *server) beginCreditAnalysisJob(ctx context.Context, season markerSeason, total int) int64 {
	message := fmt.Sprintf("Preparando finais · %s · temporada %d · lote de %d/%d episódio(s)", season.SeriesTitle, season.Season, total, season.Size)
	result, err := s.db.ExecContext(ctx, `
INSERT INTO credit_analysis_jobs(library_id,series_key,series_title,season_number,status,progress,total,processed,detected,failed,message,started_at)
VALUES(?,?,?,?,'running',1,?,0,0,0,?,CURRENT_TIMESTAMP)`, season.LibraryID, season.SeriesKey, season.SeriesTitle, season.Season, total, message)
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	s.admin.Log(ctx, "info", "markers", "Detecção automática de créditos iniciada", nil, fmt.Sprintf("%s · temporada %d · %d episódio(s)", season.SeriesTitle, season.Season, total))
	return id
}

func (s *server) updateCreditAnalysisJob(ctx context.Context, jobID int64, progress, processed, detected, failed int, message string) {
	if jobID <= 0 {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 99 {
		progress = 99
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE credit_analysis_jobs SET progress=?,processed=?,detected=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, progress, processed, detected, failed, message, jobID)
}

func (s *server) finishCreditAnalysisJob(ctx context.Context, jobID int64, season markerSeason, status string, processed, detected, failed int, message string) {
	if jobID <= 0 {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE credit_analysis_jobs SET status=?,progress=100,processed=?,detected=?,failed=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, processed, detected, failed, message, jobID)
	level := "info"
	if status == "error" || status == "completed_with_errors" {
		level = "warn"
	}
	s.admin.Log(ctx, level, "markers", "Detecção automática de créditos finalizada", nil, fmt.Sprintf("%s · temporada %d · episódios detectados %d · erros %d · %s", season.SeriesTitle, season.Season, detected, failed, message))
}

func (s *server) waitForPlaybackBeforeCreditDecode(ctx context.Context, jobID int64, processed, detected, failed int, season markerSeason) bool {
	for s.markerAnalysisPlaybackBusy(ctx) {
		s.updateCreditAnalysisJob(ctx, jobID, 5, processed, detected, failed, fmt.Sprintf("Pausado para priorizar uma reprodução ativa · %s · temporada %d", season.SeriesTitle, season.Season))
		if !waitCreditAnalyzer(ctx, 30*time.Second) {
			return false
		}
	}
	return true
}

func (s *server) analyzeOneCreditSeason(ctx context.Context) bool {
	season, err := s.nextCreditSeason(ctx)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	episodes, err := s.creditSeasonEpisodes(ctx, season)
	if err != nil || len(episodes) < 2 {
		return false
	}

	jobID := s.beginCreditAnalysisJob(ctx, season, len(episodes))
	usable, extractFailed := 0, 0
	for i := range episodes {
		if !s.waitForPlaybackBeforeCreditDecode(ctx, jobID, i, 0, extractFailed, season) {
			s.finishCreditAnalysisJob(context.Background(), jobID, season, "error", i, 0, extractFailed, "Interrompido durante encerramento do servidor")
			return true
		}
		progress := 5 + i*65/len(episodes)
		s.updateCreditAnalysisJob(ctx, jobID, progress, i, 0, extractFailed, fmt.Sprintf("Lendo final %d/%d · episódio %d", i+1, len(episodes), episodes[i].EpisodeNumber))
		probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		mediaAnalysisBudget.Lock()
		fingerprint, extractErr := markerdetect.ExtractCreditsFingerprint(probeCtx, episodes[i].Path, episodes[i].DurationSeconds)
		mediaAnalysisBudget.Unlock()
		cancel()
		episodes[i].Fingerprint, episodes[i].ExtractErr = fingerprint, extractErr
		if extractErr == nil && len(fingerprint.Frames) > 0 {
			usable++
		} else {
			extractFailed++
		}
		s.updateCreditAnalysisJob(ctx, jobID, 5+(i+1)*65/len(episodes), i+1, 0, extractFailed, fmt.Sprintf("Final %d/%d preparado · episódio %d", i+1, len(episodes), episodes[i].EpisodeNumber))
		select {
		case <-ctx.Done():
			s.finishCreditAnalysisJob(context.Background(), jobID, season, "error", i+1, 0, extractFailed, "Interrompido durante encerramento do servidor")
			return true
		case <-time.After(150 * time.Millisecond):
		}
	}

	if usable < 2 {
		for _, episode := range episodes {
			message := "not enough analyzable peer episodes"
			if episode.ExtractErr != nil {
				message = shortMarkerError(episode.ExtractErr)
			}
			s.storeCreditAnalysis(ctx, episode, season.Size, "error", 0, 0, message)
		}
		s.finishCreditAnalysisJob(ctx, jobID, season, "completed_with_errors", len(episodes), 0, maxIntHTTP(extractFailed, 1), "Não houve episódios analisáveis suficientes para comparar os créditos")
		return true
	}

	s.updateCreditAnalysisJob(ctx, jobID, 74, len(episodes), 0, extractFailed, fmt.Sprintf("Comparando finais · %s · temporada %d", season.SeriesTitle, season.Season))
	candidates := make(map[int64][]markerdetect.Candidate, len(episodes))
	for i := 0; i < len(episodes); i++ {
		if episodes[i].ExtractErr != nil || len(episodes[i].Fingerprint.Frames) == 0 {
			continue
		}
		for j := i + 1; j < len(episodes); j++ {
			if episodes[j].ExtractErr != nil || len(episodes[j].Fingerprint.Frames) == 0 {
				continue
			}
			for _, match := range markerdetect.FindMatches(episodes[i].Fingerprint.Frames, episodes[j].Fingerprint.Frames, 4) {
				aStart := episodes[i].Fingerprint.Offset + match.AStart
				aEnd := episodes[i].Fingerprint.Offset + match.AEnd
				bStart := episodes[j].Fingerprint.Offset + match.BStart
				bEnd := episodes[j].Fingerprint.Offset + match.BEnd
				// Be conservative: automatic credit skipping is only allowed in the
				// second half of an episode and never beyond the measured duration.
				if aStart < episodes[i].DurationSeconds/2 || bStart < episodes[j].DurationSeconds/2 || aEnd > episodes[i].DurationSeconds+0.5 || bEnd > episodes[j].DurationSeconds+0.5 {
					continue
				}
				candidates[episodes[i].ID] = append(candidates[episodes[i].ID], markerdetect.Candidate{Start: aStart, End: aEnd, Similarity: match.Similarity})
				candidates[episodes[j].ID] = append(candidates[episodes[j].ID], markerdetect.Candidate{Start: bStart, End: bEnd, Similarity: match.Similarity})
			}
		}
	}

	minimumSupport := 1
	if usable >= 3 {
		minimumSupport = 2
	}
	detectedEpisodes, totalSegments := 0, 0
	for index, episode := range episodes {
		if episode.ExtractErr != nil {
			s.storeCreditAnalysis(ctx, episode, season.Size, "error", 0, 0, shortMarkerError(episode.ExtractErr))
			continue
		}

		// A human or chapter marker remains authoritative. Automatic segmented
		// markers are removed so the player cannot present duplicate intervals.
		if episode.ExistingSource != "" && !strings.EqualFold(episode.ExistingSource, "automatic") {
			_, _ = s.db.ExecContext(ctx, `DELETE FROM media_marker_segments WHERE media_id=? AND kind='credits' AND source='automatic'`, episode.ID)
			s.storeCreditAnalysis(ctx, episode, season.Size, "detected", 1, 1, "protected by "+episode.ExistingSource+" marker")
			detectedEpisodes++
			totalSegments++
			continue
		}

		segments := markerdetect.ConsensusAll(candidates[episode.ID], minimumSupport, 4)
		if len(segments) == 0 {
			if episode.AnalysisModified != 0 && episode.AnalysisModified != episode.ModifiedUnix {
				_, _ = s.db.ExecContext(ctx, `DELETE FROM media_marker_segments WHERE media_id=? AND kind='credits' AND source='automatic'`, episode.ID)
				_, _ = s.db.ExecContext(ctx, `DELETE FROM media_markers WHERE media_id=? AND kind='credits' AND source='automatic'`, episode.ID)
			}
			s.storeCreditAnalysis(ctx, episode, season.Size, "none", 0, 0, "")
		} else {
			confidence := 1.0
			for _, segment := range segments {
				c := math.Min(0.96, 0.62+0.32*segment.Similarity)
				if c < confidence {
					confidence = c
				}
			}
			tx, txErr := s.db.BeginTx(ctx, nil)
			if txErr != nil {
				s.storeCreditAnalysis(ctx, episode, season.Size, "error", 0, 0, shortMarkerError(txErr))
				extractFailed++
				continue
			}
			_, txErr = tx.ExecContext(ctx, `DELETE FROM media_marker_segments WHERE media_id=? AND kind='credits' AND source='automatic'`, episode.ID)
			if txErr == nil {
				_, txErr = tx.ExecContext(ctx, `DELETE FROM media_markers WHERE media_id=? AND kind='credits' AND source='automatic'`, episode.ID)
			}
			for segmentIndex, segment := range segments {
				if txErr != nil {
					break
				}
				_, txErr = tx.ExecContext(ctx, `INSERT INTO media_marker_segments(media_id,kind,segment_index,start_seconds,end_seconds,source,confidence) VALUES(?,'credits',?,?,?,'automatic',?)`, episode.ID, segmentIndex, segment.Start, segment.End, confidence)
			}
			if txErr == nil {
				txErr = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
			if txErr != nil {
				s.storeCreditAnalysis(ctx, episode, season.Size, "error", 0, 0, shortMarkerError(txErr))
				extractFailed++
				continue
			}
			s.storeCreditAnalysis(ctx, episode, season.Size, "detected", confidence, len(segments), "")
			detectedEpisodes++
			totalSegments += len(segments)
		}
		progress := 80 + (index+1)*19/len(episodes)
		s.updateCreditAnalysisJob(ctx, jobID, progress, len(episodes), detectedEpisodes, extractFailed, fmt.Sprintf("Salvando resultados %d/%d · %d episódio(s) · %d segmento(s)", index+1, len(episodes), detectedEpisodes, totalSegments))
	}

	status := "completed"
	if extractFailed > 0 {
		status = "completed_with_errors"
	}
	message := fmt.Sprintf("%d episódio(s) com créditos · %d segmento(s) seguro(s)", detectedEpisodes, totalSegments)
	if extractFailed > 0 {
		message += fmt.Sprintf(" · %d erro(s) de leitura/análise", extractFailed)
	}
	s.finishCreditAnalysisJob(ctx, jobID, season, status, len(episodes), detectedEpisodes, extractFailed, message)
	return true
}

func (s *server) storeCreditAnalysis(ctx context.Context, episode creditEpisode, seasonSize int, status string, confidence float64, segmentCount int, lastError string) {
	_, _ = s.db.ExecContext(ctx, `
INSERT INTO media_credit_analysis(media_id,source_modified_unix,season_size,credit_status,credit_confidence,segment_count,analyzed_at,last_error)
VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,?)
ON CONFLICT(media_id) DO UPDATE SET
 source_modified_unix=excluded.source_modified_unix,season_size=excluded.season_size,credit_status=excluded.credit_status,
 credit_confidence=excluded.credit_confidence,segment_count=excluded.segment_count,analyzed_at=CURRENT_TIMESTAMP,last_error=excluded.last_error,updated_at=CURRENT_TIMESTAMP`,
		episode.ID, episode.ModifiedUnix, seasonSize, status, confidence, segmentCount, lastError)
}
