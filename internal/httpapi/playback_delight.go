package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type playbackPreferenceState struct {
	SkipMode                  string `json:"skip_mode"`
	RewindSeconds             int    `json:"rewind_seconds"`
	StillWatching             bool   `json:"still_watching"`
	StillWatchingEpisodeLimit int    `json:"still_watching_episode_limit"`
	StillWatchingHours        int    `json:"still_watching_hours"`
	AutoplayCountdown         int    `json:"autoplay_countdown"`
}

type playbackMarkerState struct {
	Kind         string  `json:"kind"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Source       string  `json:"source,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
}

func defaultPlaybackPreferences() playbackPreferenceState {
	return playbackPreferenceState{
		SkipMode:                  "manual",
		RewindSeconds:             10,
		StillWatching:             true,
		StillWatchingEpisodeLimit: 3,
		StillWatchingHours:        3,
		AutoplayCountdown:         10,
	}
}

func normalizePlaybackPreferences(in playbackPreferenceState) playbackPreferenceState {
	out := in
	switch strings.ToLower(strings.TrimSpace(out.SkipMode)) {
	case "manual", "automatic", "disabled":
		out.SkipMode = strings.ToLower(strings.TrimSpace(out.SkipMode))
	default:
		out.SkipMode = "manual"
	}
	switch out.RewindSeconds {
	case 0, 5, 10, 15, 30:
	default:
		out.RewindSeconds = 10
	}
	switch out.AutoplayCountdown {
	case 0, 5, 10, 15, 30:
	default:
		out.AutoplayCountdown = 10
	}
	if out.StillWatchingEpisodeLimit < 1 {
		out.StillWatchingEpisodeLimit = 1
	}
	if out.StillWatchingEpisodeLimit > 12 {
		out.StillWatchingEpisodeLimit = 12
	}
	if out.StillWatchingHours < 1 {
		out.StillWatchingHours = 1
	}
	if out.StillWatchingHours > 12 {
		out.StillWatchingHours = 12
	}
	return out
}

func (s *server) loadPlaybackPreferences(ctx context.Context, profileID int64) (playbackPreferenceState, bool, error) {
	out := defaultPlaybackPreferences()
	if profileID <= 0 {
		return out, false, nil
	}
	err := s.db.QueryRowContext(ctx, `SELECT skip_mode,rewind_seconds,still_watching,still_watching_episode_limit,still_watching_hours,autoplay_countdown FROM profile_playback_preferences WHERE profile_id=?`, profileID).
		Scan(&out.SkipMode, &out.RewindSeconds, &out.StillWatching, &out.StillWatchingEpisodeLimit, &out.StillWatchingHours, &out.AutoplayCountdown)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	return normalizePlaybackPreferences(out), true, nil
}

func (s *server) savePlaybackPreferences(ctx context.Context, profileID int64, in playbackPreferenceState) (playbackPreferenceState, error) {
	if profileID <= 0 {
		return playbackPreferenceState{}, errors.New("profile is required")
	}
	in = normalizePlaybackPreferences(in)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO profile_playback_preferences(profile_id,skip_mode,rewind_seconds,still_watching,still_watching_episode_limit,still_watching_hours,autoplay_countdown)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(profile_id) DO UPDATE SET
 skip_mode=excluded.skip_mode,rewind_seconds=excluded.rewind_seconds,still_watching=excluded.still_watching,
 still_watching_episode_limit=excluded.still_watching_episode_limit,still_watching_hours=excluded.still_watching_hours,
 autoplay_countdown=excluded.autoplay_countdown,updated_at=CURRENT_TIMESTAMP`,
		profileID, in.SkipMode, in.RewindSeconds, in.StillWatching, in.StillWatchingEpisodeLimit, in.StillWatchingHours, in.AutoplayCountdown)
	return in, err
}

func validMarkerKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "intro", "credits", "recap":
		return true
	default:
		return false
	}
}

func (s *server) loadMediaMarkers(ctx context.Context, mediaID int64) ([]playbackMarkerState, error) {
	legacy := []playbackMarkerState{}
	rows, err := s.db.QueryContext(ctx, `SELECT kind,start_seconds,end_seconds,source,confidence FROM media_markers WHERE media_id=? ORDER BY start_seconds,kind`, mediaID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var marker playbackMarkerState
		if err := rows.Scan(&marker.Kind, &marker.StartSeconds, &marker.EndSeconds, &marker.Source, &marker.Confidence); err != nil {
			_ = rows.Close()
			return nil, err
		}
		legacy = append(legacy, marker)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	segments := []playbackMarkerState{}
	segmentRows, err := s.db.QueryContext(ctx, `SELECT kind,start_seconds,end_seconds,source,confidence FROM media_marker_segments WHERE media_id=? ORDER BY start_seconds,segment_index`, mediaID)
	if err != nil {
		return nil, err
	}
	for segmentRows.Next() {
		var marker playbackMarkerState
		if err := segmentRows.Scan(&marker.Kind, &marker.StartSeconds, &marker.EndSeconds, &marker.Source, &marker.Confidence); err != nil {
			_ = segmentRows.Close()
			return nil, err
		}
		segments = append(segments, marker)
	}
	if err := segmentRows.Err(); err != nil {
		_ = segmentRows.Close()
		return nil, err
	}
	_ = segmentRows.Close()

	protectedCredits := false
	for _, marker := range legacy {
		if marker.Kind == "credits" && !strings.EqualFold(marker.Source, "automatic") {
			protectedCredits = true
			break
		}
	}
	out := make([]playbackMarkerState, 0, len(legacy)+len(segments))
	for _, marker := range legacy {
		if marker.Kind == "credits" && strings.EqualFold(marker.Source, "automatic") && len(segments) > 0 {
			continue
		}
		out = append(out, marker)
	}
	if !protectedCredits {
		out = append(out, segments...)
	}
	return out, nil
}

func normalizeMarkerSource(source string) (string, float64) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "chapter":
		return "chapter", 0.8
	case "automatic":
		return "automatic", 0.7
	default:
		return "manual", 1
	}
}

func (s *server) saveMediaMarker(ctx context.Context, mediaID int64, marker playbackMarkerState) error {
	marker.Kind = strings.ToLower(strings.TrimSpace(marker.Kind))
	if !validMarkerKind(marker.Kind) {
		return errors.New("invalid marker kind")
	}
	if marker.StartSeconds < 0 || marker.EndSeconds <= marker.StartSeconds || marker.EndSeconds > 86400 {
		return errors.New("invalid marker interval")
	}
	source, confidence := normalizeMarkerSource(marker.Source)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO media_markers(media_id,kind,start_seconds,end_seconds,source,confidence)
VALUES(?,?,?,?,?,?)
ON CONFLICT(media_id,kind) DO UPDATE SET
 start_seconds=excluded.start_seconds,end_seconds=excluded.end_seconds,source=excluded.source,confidence=excluded.confidence,updated_at=CURRENT_TIMESTAMP
WHERE media_markers.source<>'manual' OR excluded.source='manual'`,
		mediaID, marker.Kind, marker.StartSeconds, marker.EndSeconds, source, confidence)
	if err != nil {
		return err
	}
	if marker.Kind == "credits" && source != "automatic" {
		_, err = s.db.ExecContext(ctx, `DELETE FROM media_marker_segments WHERE media_id=? AND kind='credits' AND source='automatic'`, mediaID)
	}
	return err
}

func (s *server) deleteMediaMarker(ctx context.Context, mediaID int64, kind string) error {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if !validMarkerKind(kind) {
		return errors.New("invalid marker kind")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM media_markers WHERE media_id=? AND kind=?`, mediaID, kind); err != nil {
		_ = tx.Rollback()
		return err
	}
	if kind == "credits" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM media_marker_segments WHERE media_id=? AND kind='credits'`, mediaID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *server) handlePlaybackDelightTelemetry(w http.ResponseWriter, r *http.Request, mediaID int64, in playbackTelemetryInput) bool {
	op := strings.ToLower(strings.TrimSpace(in.Operation))
	if op == "" {
		return false
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)

	switch op {
	case "playback_state_get":
		prefs, persisted, err := s.loadPlaybackPreferences(r.Context(), profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return true
		}
		markers, err := s.loadMediaMarkers(r.Context(), mediaID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "profile_id": profileID, "playback_preferences": prefs,
			"playback_preferences_persisted": persisted, "markers": markers,
		})
		return true
	case "playback_preferences_set":
		if in.PlaybackPreferences == nil {
			writeError(w, http.StatusBadRequest, errors.New("playback_preferences is required"))
			return true
		}
		prefs, err := s.savePlaybackPreferences(r.Context(), profileID, *in.PlaybackPreferences)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "playback_preferences": prefs})
		return true
	case "marker_upsert":
		if in.Marker == nil {
			writeError(w, http.StatusBadRequest, errors.New("marker is required"))
			return true
		}
		if err := s.saveMediaMarker(r.Context(), mediaID, *in.Marker); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return true
	case "marker_delete":
		if in.Marker == nil {
			writeError(w, http.StatusBadRequest, errors.New("marker is required"))
			return true
		}
		if err := s.deleteMediaMarker(r.Context(), mediaID, in.Marker.Kind); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return true
	default:
		writeError(w, http.StatusBadRequest, errors.New("unsupported playback operation"))
		return true
	}
}
