package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/playback"
)

type technicalSnapshot struct {
	MediaID           int64            `json:"media_id"`
	VideoCodec        string           `json:"video_codec"`
	Width             int              `json:"width"`
	Height            int              `json:"height"`
	HDR               string           `json:"hdr"`
	BitrateKbps       int64            `json:"bitrate_kbps"`
	DurationSeconds   float64          `json:"duration_seconds"`
	AudioLanguages    []string         `json:"audio_languages"`
	SubtitleLanguages []string         `json:"subtitle_languages"`
	AudioPTBR         bool             `json:"audio_pt_br"`
	SubtitlePTBR      bool             `json:"subtitle_pt_br"`
	DubStatus         string           `json:"dub_status"`
	Status            string           `json:"status"`
	LastError         string           `json:"last_error,omitempty"`
	Source            playback.Source  `json:"source"`
}

var technicalIndexerKick = make(chan struct{}, 1)
var technicalIndexerOnce sync.Once

// startTechnicalIndexer keeps the technical catalog warm without slowing the
// scanner. Only one ffprobe runs at a time, which is intentional for remote
// rclone/Drive mounts. A changed modified_unix invalidates the cached probe.
func (s *server) startTechnicalIndexer(ctx context.Context) {
	technicalIndexerOnce.Do(func() {
		go func() {
			for {
				worked := s.indexOneTechnicalItem(ctx)
				if worked {
					select {
					case <-ctx.Done():
						return
					case <-time.After(250 * time.Millisecond):
					}
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-technicalIndexerKick:
				case <-time.After(45 * time.Second):
				}
			}
		}()
	})
}

func (s *server) kickTechnicalIndexer() {
	select {
	case technicalIndexerKick <- struct{}{}:
	default:
	}
}

func (s *server) indexOneTechnicalItem(ctx context.Context) bool {
	var id, modified int64
	var path string
	err := s.db.QueryRowContext(ctx, `
SELECT m.id,m.path,m.modified_unix
FROM media m JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_technical mt ON mt.media_id=m.id
WHERE m.available=1 AND l.kind<>'music'
  AND (
    mt.media_id IS NULL
    OR mt.source_modified_unix<>m.modified_unix
    OR mt.status='pending'
    OR (mt.status='error' AND (mt.probed_at IS NULL OR mt.probed_at<=datetime('now','-30 minutes')))
  )
ORDER BY CASE WHEN mt.media_id IS NULL THEN 0 WHEN mt.status='pending' THEN 1 ELSE 2 END,m.id
LIMIT 1`).Scan(&id, &path, &modified)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	_, _ = s.probeAndStoreTechnical(probeCtx, id, path, modified, true)
	return true
}

func (s *server) probeMediaSource(ctx context.Context, mediaID int64, path string, modifiedUnix int64) (playback.Source, error) {
	var raw, status string
	var cachedModified int64
	err := s.db.QueryRowContext(ctx, `SELECT source_json,status,source_modified_unix FROM media_technical WHERE media_id=?`, mediaID).Scan(&raw, &status, &cachedModified)
	if err == nil && status == "ok" && cachedModified == modifiedUnix && strings.TrimSpace(raw) != "" {
		var source playback.Source
		if json.Unmarshal([]byte(raw), &source) == nil && len(source.Streams) > 0 {
			return source, nil
		}
	}
	// The technical cache is an optimization, not a playback prerequisite. A
	// successful probe must remain usable even if its auxiliary cache write is
	// temporarily blocked by SQLite contention; the indexer can persist it later.
	return s.probeAndStoreTechnical(ctx, mediaID, path, modifiedUnix, true)
}

func (s *server) probeAndStoreTechnical(ctx context.Context, mediaID int64, path string, modifiedUnix int64, bestEffortStore bool) (playback.Source, error) {
	source, err := playback.Probe(ctx, path)
	if err != nil {
		_, _ = s.db.ExecContext(context.Background(), `
INSERT INTO media_technical(media_id,source_modified_unix,status,last_error,updated_at,probed_at)
VALUES(?,?,'error',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET source_modified_unix=excluded.source_modified_unix,status='error',last_error=excluded.last_error,updated_at=CURRENT_TIMESTAMP,probed_at=CURRENT_TIMESTAMP`, mediaID, modifiedUnix, err.Error())
		return playback.Source{}, err
	}
	snapshot := technicalFromSource(mediaID, source)
	raw, _ := json.Marshal(source)
	audioJSON, _ := json.Marshal(snapshot.AudioLanguages)
	subtitleJSON, _ := json.Marshal(snapshot.SubtitleLanguages)
	_, saveErr := s.db.ExecContext(context.Background(), `
INSERT INTO media_technical(media_id,source_modified_unix,source_json,video_codec,width,height,hdr,bitrate_kbps,duration_seconds,audio_json,subtitle_json,audio_pt_br,subtitle_pt_br,dub_status,status,last_error,probed_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'ok','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)
ON CONFLICT(media_id) DO UPDATE SET
 source_modified_unix=excluded.source_modified_unix,source_json=excluded.source_json,video_codec=excluded.video_codec,width=excluded.width,height=excluded.height,hdr=excluded.hdr,
 bitrate_kbps=excluded.bitrate_kbps,duration_seconds=excluded.duration_seconds,audio_json=excluded.audio_json,subtitle_json=excluded.subtitle_json,
 audio_pt_br=excluded.audio_pt_br,subtitle_pt_br=excluded.subtitle_pt_br,dub_status=excluded.dub_status,status='ok',last_error='',probed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
		mediaID, modifiedUnix, string(raw), snapshot.VideoCodec, snapshot.Width, snapshot.Height, snapshot.HDR, snapshot.BitrateKbps, snapshot.DurationSeconds,
		string(audioJSON), string(subtitleJSON), snapshot.AudioPTBR, snapshot.SubtitlePTBR, snapshot.DubStatus)
	if saveErr != nil && !bestEffortStore {
		return playback.Source{}, saveErr
	}
	return source, nil
}

func technicalFromSource(mediaID int64, source playback.Source) technicalSnapshot {
	out := technicalSnapshot{MediaID: mediaID, BitrateKbps: source.BitrateKbps, DurationSeconds: source.DurationSeconds, Status: "ok", Source: source}
	audioSet := map[string]bool{}
	subtitleSet := map[string]bool{}
	for _, stream := range source.Streams {
		lang := normalizedTechnicalLanguage(stream.Language, stream.Title)
		switch stream.Type {
		case "video":
			if out.VideoCodec == "" {
				out.VideoCodec = stream.Codec
				out.Width, out.Height, out.HDR = stream.Width, stream.Height, stream.HDR
			}
		case "audio":
			if lang != "" {
				audioSet[lang] = true
			}
			if isPortugueseTechnical(stream.Language, stream.Title) {
				out.AudioPTBR = true
			}
		case "subtitle":
			if lang != "" {
				subtitleSet[lang] = true
			}
			if isPortugueseTechnical(stream.Language, stream.Title) {
				out.SubtitlePTBR = true
			}
		}
	}
	for lang := range audioSet {
		out.AudioLanguages = append(out.AudioLanguages, lang)
	}
	for lang := range subtitleSet {
		out.SubtitleLanguages = append(out.SubtitleLanguages, lang)
	}
	sort.Strings(out.AudioLanguages)
	sort.Strings(out.SubtitleLanguages)
	switch {
	case out.AudioPTBR:
		out.DubStatus = "dublado"
	case out.SubtitlePTBR:
		out.DubStatus = "legendado"
	default:
		out.DubStatus = "original"
	}
	return out
}

func normalizedTechnicalLanguage(language, title string) string {
	value := strings.ToLower(strings.TrimSpace(language))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "por", "pt", "pt-br", "ptbr", "pob", "pb":
		return "pt-BR"
	case "jpn", "ja", "jp":
		return "ja"
	case "eng", "en":
		return "en"
	case "spa", "es":
		return "es"
	}
	lowTitle := strings.ToLower(title)
	if strings.Contains(lowTitle, "portugu") || strings.Contains(lowTitle, "dublado") || strings.Contains(lowTitle, "brasil") {
		return "pt-BR"
	}
	if value == "und" || value == "unknown" {
		return ""
	}
	return value
}

func isPortugueseTechnical(language, title string) bool {
	return normalizedTechnicalLanguage(language, title) == "pt-BR"
}

func (s *server) technicalSnapshotFor(ctx context.Context, mediaID int64) (technicalSnapshot, bool) {
	var snapshot technicalSnapshot
	var audioJSON, subtitleJSON, sourceJSON string
	err := s.db.QueryRowContext(ctx, `SELECT media_id,video_codec,width,height,hdr,bitrate_kbps,duration_seconds,audio_json,subtitle_json,audio_pt_br,subtitle_pt_br,dub_status,status,last_error,source_json FROM media_technical WHERE media_id=?`, mediaID).
		Scan(&snapshot.MediaID, &snapshot.VideoCodec, &snapshot.Width, &snapshot.Height, &snapshot.HDR, &snapshot.BitrateKbps, &snapshot.DurationSeconds, &audioJSON, &subtitleJSON, &snapshot.AudioPTBR, &snapshot.SubtitlePTBR, &snapshot.DubStatus, &snapshot.Status, &snapshot.LastError, &sourceJSON)
	if err != nil {
		return technicalSnapshot{}, false
	}
	_ = json.Unmarshal([]byte(audioJSON), &snapshot.AudioLanguages)
	_ = json.Unmarshal([]byte(subtitleJSON), &snapshot.SubtitleLanguages)
	_ = json.Unmarshal([]byte(sourceJSON), &snapshot.Source)
	return snapshot, true
}

func (s *server) technicalCatalogStatus(w http.ResponseWriter, r *http.Request) {
	var total, ready, pending, failed int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id WHERE m.available=1 AND l.kind<>'music'`).Scan(&total)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_technical mt JOIN media m ON m.id=mt.media_id WHERE m.available=1 AND mt.status='ok' AND mt.source_modified_unix=m.modified_unix`).Scan(&ready)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_technical mt JOIN media m ON m.id=mt.media_id WHERE m.available=1 AND mt.status='error' AND mt.source_modified_unix=m.modified_unix`).Scan(&failed)
	pending = total - ready - failed
	if pending < 0 {
		pending = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "ready": ready, "pending": pending, "failed": failed, "automatic": true})
}

func (s *server) restartTechnicalCatalog(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.ExecContext(r.Context(), `UPDATE media_technical SET status='pending',updated_at=CURRENT_TIMESTAMP WHERE media_id IN (SELECT id FROM media WHERE available=1)`); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.kickTechnicalIndexer()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "análise técnica colocada em segundo plano"})
}
