package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/markerdetect"
)

var markerAnalyzerOnce sync.Once
var markerAnalyzerKick = make(chan struct{}, 1)

// mediaAnalysisBudget serializes expensive ffmpeg/ffprobe work owned by this
// package. The technical indexer also uses this lock, so background marker
// discovery cannot pile another decoder on top of catalog probing.
var mediaAnalysisBudget sync.Mutex

type markerSeason struct {
	LibraryID  int64
	Library    string
	SeriesKey  string
	SeriesTitle string
	Season     int
	Size       int
}

type markerEpisode struct {
	ID               int64
	Path             string
	ModifiedUnix     int64
	DurationSeconds  float64
	EpisodeNumber    int
	AnalysisModified int64
	ExistingSource   string
	Frames           []markerdetect.Frame
	ExtractErr       error
}

func (s *server) startMarkerAnalyzer(ctx context.Context) {
	markerAnalyzerOnce.Do(func() {
		// A process restart cannot resume an ffmpeg pipe halfway through. Keep the
		// operational history honest and let the normal selector retry the season.
		_, _ = s.db.ExecContext(context.Background(), `
UPDATE marker_analysis_jobs
SET status='error',progress=100,message='Interrompido por reinício do servidor; a temporada será analisada novamente.',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
WHERE status='running'`)
		go func() {
			// Let startup, migrations, the first Home and the technical indexer get
			// ahead before audio comparison starts.
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Second):
			}
			for {
				if s.markerAnalysisPlaybackBusy(ctx) {
					if !waitMarkerAnalyzer(ctx, 30*time.Second) {
						return
					}
					continue
				}
				worked := s.analyzeOneIntroSeason(ctx)
				if worked {
					if !waitMarkerAnalyzer(ctx, 8*time.Second) {
						return
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-markerAnalyzerKick:
				case <-time.After(60 * time.Second):
				}
			}
		}()
	})
}

func waitMarkerAnalyzer(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-markerAnalyzerKick:
		return true
	case <-time.After(delay):
		return true
	}
}

func (s *server) kickMarkerAnalyzer() {
	select {
	case markerAnalyzerKick <- struct{}{}:
	default:
	}
}

func (s *server) markerAnalysisPlaybackBusy(ctx context.Context) bool {
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_sessions WHERE last_seen_at>=datetime('now','-90 seconds')`).Scan(&active)
	return err == nil && active > 0
}

func (s *server) nextIntroSeason(ctx context.Context) (markerSeason, error) {
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
LEFT JOIN media_marker_analysis ma ON ma.media_id=m.id
WHERE m.available=1
  AND si.series_key<>''
  AND si.episode_number>0
  AND mt.status='ok'
  AND mt.source_modified_unix=m.modified_unix
  AND mt.duration_seconds>=40
  AND (
       ma.media_id IS NULL
       OR ma.source_modified_unix<>m.modified_unix
       OR ma.season_size<>(SELECT COUNT(*)
            FROM media sm
            JOIN media_series_identity ssi ON ssi.media_id=sm.id
           WHERE sm.available=1
             AND ssi.library_id=si.library_id
             AND ssi.series_key=si.series_key
             AND ssi.season_number=si.season_number
             AND ssi.episode_number>0)
       OR ma.intro_status='pending'
       OR (ma.intro_status='error' AND (ma.analyzed_at IS NULL OR ma.analyzed_at<=datetime('now','-6 hours')))
  )
ORDER BY CASE WHEN ma.media_id IS NULL THEN 0 WHEN ma.intro_status='pending' THEN 1 ELSE 2 END,m.id
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

func (s *server) introSeasonEpisodes(ctx context.Context, season markerSeason) ([]markerEpisode, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id,m.path,m.modified_unix,mt.duration_seconds,si.episode_number,
       COALESCE(ma.source_modified_unix,0),COALESCE(mm.source,'')
FROM media m
JOIN media_series_identity si ON si.media_id=m.id
JOIN media_technical mt ON mt.media_id=m.id
LEFT JOIN media_marker_analysis ma ON ma.media_id=m.id
LEFT JOIN media_markers mm ON mm.media_id=m.id AND mm.kind='intro'
WHERE m.available=1
  AND si.library_id=? AND si.series_key=? AND si.season_number=? AND si.episode_number>0
  AND mt.status='ok' AND mt.source_modified_unix=m.modified_unix AND mt.duration_seconds>=40
ORDER BY CASE
           WHEN ma.media_id IS NULL OR ma.source_modified_unix<>m.modified_unix OR ma.season_size<>? OR ma.intro_status='pending' THEN 0
           ELSE 1
         END,
         si.episode_number,m.id
LIMIT 12`, season.LibraryID, season.SeriesKey, season.Season, season.Size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []markerEpisode{}
	for rows.Next() {
		var episode markerEpisode
		if err := rows.Scan(&episode.ID, &episode.Path, &episode.ModifiedUnix, &episode.DurationSeconds, &episode.EpisodeNumber, &episode.AnalysisModified, &episode.ExistingSource); err != nil {
			return nil, err
		}
		out = append(out, episode)
	}
	return out, rows.Err()
}

func (s *server) beginIntroAnalysisJob(ctx context.Context, season markerSeason, total int) int64 {
	message := fmt.Sprintf("Preparando fingerprints · %s · temporada %d · lote de %d/%d episódio(s)", season.SeriesTitle, season.Season, total, season.Size)
	result, err := s.db.ExecContext(ctx, `
INSERT INTO marker_analysis_jobs(library_id,series_key,series_title,season_number,status,progress,total,processed,detected,failed,message,started_at)
VALUES(?,?,?,?,'running',1,?,0,0,0,?,CURRENT_TIMESTAMP)`, season.LibraryID, season.SeriesKey, season.SeriesTitle, season.Season, total, message)
	if err != nil {
		return 0
	}
	id, _ := result.LastInsertId()
	s.admin.Log(ctx, "info", "markers", "Detecção automática de introduções iniciada", nil, fmt.Sprintf("%s · temporada %d · %d episódio(s)", season.SeriesTitle, season.Season, total))
	return id
}

func (s *server) updateIntroAnalysisJob(ctx context.Context, jobID int64, progress, processed, detected, failed int, message string) {
	if jobID <= 0 {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 99 {
		progress = 99
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE marker_analysis_jobs SET progress=?,processed=?,detected=?,failed=?,message=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='running'`, progress, processed, detected, failed, message, jobID)
}

func (s *server) finishIntroAnalysisJob(ctx context.Context, jobID int64, season markerSeason, status string, processed, detected, failed int, message string) {
	if jobID <= 0 {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE marker_analysis_jobs SET status=?,progress=100,processed=?,detected=?,failed=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, processed, detected, failed, message, jobID)
	level := "info"
	if status == "error" || status == "completed_with_errors" {
		level = "warn"
	}
	s.admin.Log(ctx, level, "markers", "Detecção automática de introduções finalizada", nil, fmt.Sprintf("%s · temporada %d · detectadas %d · erros %d · %s", season.SeriesTitle, season.Season, detected, failed, message))
}

func (s *server) waitForPlaybackBeforeMarkerDecode(ctx context.Context, jobID int64, processed, detected, failed int, season markerSeason) bool {
	for s.markerAnalysisPlaybackBusy(ctx) {
		s.updateIntroAnalysisJob(ctx, jobID, 5, processed, detected, failed, fmt.Sprintf("Pausado para priorizar uma reprodução ativa · %s · temporada %d", season.SeriesTitle, season.Season))
		if !waitMarkerAnalyzer(ctx, 30*time.Second) {
			return false
		}
	}
	return true
}

func (s *server) analyzeOneIntroSeason(ctx context.Context) bool {
	season, err := s.nextIntroSeason(ctx)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	episodes, err := s.introSeasonEpisodes(ctx, season)
	if err != nil || len(episodes) < 2 {
		return false
	}

	jobID := s.beginIntroAnalysisJob(ctx, season, len(episodes))
	usable, extractFailed := 0, 0
	for i := range episodes {
		if !s.waitForPlaybackBeforeMarkerDecode(ctx, jobID, i, 0, extractFailed, season) {
			s.finishIntroAnalysisJob(context.Background(), jobID, season, "error", i, 0, extractFailed, "Interrompido durante encerramento do servidor")
			return true
		}
		progress := 5 + i*65/len(episodes)
		s.updateIntroAnalysisJob(ctx, jobID, progress, i, 0, extractFailed, fmt.Sprintf("Extraindo fingerprint %d/%d · episódio %d", i+1, len(episodes), episodes[i].EpisodeNumber))
		probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		mediaAnalysisBudget.Lock()
		frames, extractErr := markerdetect.ExtractIntroFingerprint(probeCtx, episodes[i].Path, episodes[i].DurationSeconds)
		mediaAnalysisBudget.Unlock()
		cancel()
		episodes[i].Frames, episodes[i].ExtractErr = frames, extractErr
		if extractErr == nil && len(frames) > 0 {
			usable++
		} else {
			extractFailed++
		}
		s.updateIntroAnalysisJob(ctx, jobID, 5+(i+1)*65/len(episodes), i+1, 0, extractFailed, fmt.Sprintf("Fingerprint %d/%d concluído · episódio %d", i+1, len(episodes), episodes[i].EpisodeNumber))
		select {
		case <-ctx.Done():
			s.finishIntroAnalysisJob(context.Background(), jobID, season, "error", i+1, 0, extractFailed, "Interrompido durante encerramento do servidor")
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
			s.storeIntroAnalysis(ctx, episode, season.Size, "error", 0, message)
		}
		s.finishIntroAnalysisJob(ctx, jobID, season, "completed_with_errors", len(episodes), 0, maxIntHTTP(extractFailed, 1), "Não houve episódios analisáveis suficientes para comparar a abertura")
		return true
	}

	s.updateIntroAnalysisJob(ctx, jobID, 74, len(episodes), 0, extractFailed, fmt.Sprintf("Comparando fingerprints · %s · temporada %d", season.SeriesTitle, season.Season))
	candidates := make(map[int64][]markerdetect.Candidate, len(episodes))
	for i := 0; i < len(episodes); i++ {
		if episodes[i].ExtractErr != nil || len(episodes[i].Frames) == 0 {
			continue
		}
		for j := i + 1; j < len(episodes); j++ {
			if episodes[j].ExtractErr != nil || len(episodes[j].Frames) == 0 {
				continue
			}
			match, ok := markerdetect.BestMatch(episodes[i].Frames, episodes[j].Frames)
			if !ok {
				continue
			}
			if match.AEnd > episodes[i].DurationSeconds/2+0.5 || match.BEnd > episodes[j].DurationSeconds/2+0.5 {
				continue
			}
			candidates[episodes[i].ID] = append(candidates[episodes[i].ID], markerdetect.Candidate{Start: match.AStart, End: match.AEnd, Similarity: match.Similarity})
			candidates[episodes[j].ID] = append(candidates[episodes[j].ID], markerdetect.Candidate{Start: match.BStart, End: match.BEnd, Similarity: match.Similarity})
		}
	}

	minimumSupport := 1
	if usable >= 3 {
		minimumSupport = 2
	}
	detected := 0
	for index, episode := range episodes {
		if episode.ExtractErr != nil {
			s.storeIntroAnalysis(ctx, episode, season.Size, "error", 0, shortMarkerError(episode.ExtractErr))
			continue
		}
		candidate, ok := markerdetect.Consensus(candidates[episode.ID], minimumSupport)
		if !ok || candidate.Start < 0 || candidate.End <= candidate.Start || candidate.End > episode.DurationSeconds/2+0.5 {
			if episode.AnalysisModified != 0 && episode.AnalysisModified != episode.ModifiedUnix && strings.EqualFold(episode.ExistingSource, "automatic") {
				_, _ = s.db.ExecContext(ctx, `DELETE FROM media_markers WHERE media_id=? AND kind='intro' AND source='automatic'`, episode.ID)
			}
			s.storeIntroAnalysis(ctx, episode, season.Size, "none", 0, "")
		} else {
			support := len(candidates[episode.ID])
			confidence := math.Min(0.96, 0.55+0.06*float64(minIntHTTP(support, 4))+0.25*candidate.Similarity)
			_, _ = s.db.ExecContext(ctx, `
INSERT INTO media_markers(media_id,kind,start_seconds,end_seconds,source,confidence)
VALUES(?,'intro',?,?,'automatic',?)
ON CONFLICT(media_id,kind) DO UPDATE SET
 start_seconds=excluded.start_seconds,end_seconds=excluded.end_seconds,source='automatic',confidence=excluded.confidence,updated_at=CURRENT_TIMESTAMP
WHERE media_markers.source='automatic'`, episode.ID, candidate.Start, candidate.End, confidence)
			s.storeIntroAnalysis(ctx, episode, season.Size, "detected", confidence, "")
			detected++
		}
		progress := 80 + (index+1)*19/len(episodes)
		s.updateIntroAnalysisJob(ctx, jobID, progress, len(episodes), detected, extractFailed, fmt.Sprintf("Salvando resultados %d/%d · %d intro(s) detectada(s)", index+1, len(episodes), detected))
	}

	status := "completed"
	if extractFailed > 0 {
		status = "completed_with_errors"
	}
	withoutMatch := len(episodes) - extractFailed - detected
	message := fmt.Sprintf("%d intro(s) detectada(s) · %d sem correspondência", detected, maxIntHTTP(withoutMatch, 0))
	if extractFailed > 0 {
		message += fmt.Sprintf(" · %d erro(s) de leitura/análise", extractFailed)
	}
	s.finishIntroAnalysisJob(ctx, jobID, season, status, len(episodes), detected, extractFailed, message)
	return true
}

func (s *server) storeIntroAnalysis(ctx context.Context, episode markerEpisode, seasonSize int, status string, confidence float64, lastError string) {
	_, _ = s.db.ExecContext(ctx, `
INSERT INTO media_marker_analysis(media_id,source_modified_unix,season_size,intro_status,intro_confidence,analyzed_at,last_error)
VALUES(?,?,?,?,?,CURRENT_TIMESTAMP,?)
ON CONFLICT(media_id) DO UPDATE SET
 source_modified_unix=excluded.source_modified_unix,season_size=excluded.season_size,intro_status=excluded.intro_status,
 intro_confidence=excluded.intro_confidence,analyzed_at=CURRENT_TIMESTAMP,last_error=excluded.last_error,updated_at=CURRENT_TIMESTAMP`,
		episode.ID, episode.ModifiedUnix, seasonSize, status, confidence, lastError)
}

func shortMarkerError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func minIntHTTP(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntHTTP(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *server) markerAnalysisStatus(w http.ResponseWriter, r *http.Request) {
	var eligible, detected, noMatch, pending, failed, running int
	_ = s.db.QueryRowContext(r.Context(), `
SELECT COUNT(*) FROM media m
JOIN media_series_identity si ON si.media_id=m.id
JOIN media_technical mt ON mt.media_id=m.id
WHERE m.available=1 AND si.series_key<>'' AND si.episode_number>0
  AND mt.status='ok' AND mt.source_modified_unix=m.modified_unix AND mt.duration_seconds>=40`).Scan(&eligible)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_marker_analysis WHERE intro_status='detected'`).Scan(&detected)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_marker_analysis WHERE intro_status='none'`).Scan(&noMatch)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_marker_analysis WHERE intro_status='pending'`).Scan(&pending)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_marker_analysis WHERE intro_status='error'`).Scan(&failed)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM marker_analysis_jobs WHERE status='running'`).Scan(&running)
	_, ffmpegErr := exec.LookPath("ffmpeg")
	writeJSON(w, http.StatusOK, map[string]any{
		"automatic": true,
		"ready": ffmpegErr == nil,
		"eligible_episodes": eligible,
		"detected": detected,
		"no_match": noMatch,
		"pending": pending,
		"failed": failed,
		"running_jobs": running,
		"pauses_for_playback": true,
		"method": "season_audio_fingerprint",
	})
}

func (s *server) restartMarkerAnalysis(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.ExecContext(r.Context(), `
UPDATE media_marker_analysis
SET intro_status='pending',last_error='',updated_at=CURRENT_TIMESTAMP
WHERE media_id IN (
  SELECT m.id FROM media m JOIN media_series_identity si ON si.media_id=m.id
  WHERE m.available=1 AND si.series_key<>'' AND si.episode_number>0
)`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.kickMarkerAnalyzer()
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "markers", "Reanálise automática de introduções solicitada", &uid, "Todas as temporadas elegíveis foram marcadas para nova análise")
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "detecção automática de introduções colocada em segundo plano"})
}
