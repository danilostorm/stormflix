package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/streaming"
	"github.com/danilostorm/stormflix/internal/transcode"
)

type playbackHeartbeatInput struct {
	PositionSeconds  float64 `json:"position_seconds"`
	DurationSeconds  float64 `json:"duration_seconds"`
	State            string  `json:"state"`
	Mode             string  `json:"mode"`
	Resolution       string  `json:"resolution"`
	VideoCodec       string  `json:"video_codec"`
	AudioCodec       string  `json:"audio_codec"`
	SourceAudioCodec string  `json:"source_audio_codec"`
	AudioLanguage    string  `json:"audio_language"`
	SubtitleLanguage string  `json:"subtitle_language"`
	BitrateKbps      int64   `json:"bitrate_kbps"`
	PlaybackSession  string  `json:"playback_session_id"`
	ProgressSequence int64   `json:"progress_sequence"`
	ProgressEventMS  int64   `json:"progress_event_ms"`
	ProgressReason   string  `json:"progress_reason"`
}

func (s *server) playbackHeartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	u := currentUser(r)
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	var in playbackHeartbeatInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if session := normalizePlaybackSessionID(in.PlaybackSession); session != "" {
		switch {
		case streaming.IsSessionID(session):
			if manager, managerErr := streaming.ForDataDir(s.config.DataDir); managerErr == nil {
				manager.Touch(u.ID, session)
			}
		case transcode.IsSessionID(session):
			if manager, managerErr := transcode.ForDataDir(s.config.DataDir); managerErr == nil {
				manager.Touch(u.ID, session)
			}
		default:
			s.hlsCache.TouchSession(u.ID, session)
		}
	}
	hb := admin.PlaybackHeartbeat{
		PositionSeconds:  in.PositionSeconds,
		DurationSeconds:  in.DurationSeconds,
		State:            in.State,
		Mode:             in.Mode,
		Resolution:       in.Resolution,
		VideoCodec:       in.VideoCodec,
		AudioCodec:       in.AudioCodec,
		SourceAudioCodec: in.SourceAudioCodec,
		AudioLanguage:    in.AudioLanguage,
		SubtitleLanguage: in.SubtitleLanguage,
		BitrateKbps:      in.BitrateKbps,
	}
	device := shortDevice(r.UserAgent())
	if err := s.admin.Heartbeat(r.Context(), u.ID, id, device, clientIP(r), hb); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	profileID := s.selectedProfileID(r, u.ID)
	accepted := true
	if profileID > 0 {
		if in.PlaybackSession != "" && in.ProgressSequence > 0 && in.ProgressEventMS > 0 {
			accepted, err = s.admin.SaveProfileProgressOrdered(r.Context(), profileID, id, in.PositionSeconds, in.DurationSeconds, in.PlaybackSession, in.ProgressSequence, in.ProgressEventMS)
		} else {
			err = s.admin.SaveProfileProgress(r.Context(), profileID, id, in.PositionSeconds, in.DurationSeconds)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	reason := strings.TrimSpace(in.ProgressReason)
	if reason != "" && reason != "periodic" && reason != "ready" {
		level := "info"
		message := "PROGRESS_SAVE_ACCEPTED"
		if !accepted {
			level = "warn"
			message = "PROGRESS_SAVE_REJECTED_STALE"
		}
		s.admin.Log(r.Context(), level, "playback", message, &u.ID, fmt.Sprintf(
			"media=%d session=%s seq=%d event_ms=%d reason=%s position=%.3f duration=%.3f",
			id, in.PlaybackSession, in.ProgressSequence, in.ProgressEventMS, reason, in.PositionSeconds, in.DurationSeconds,
		))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "progress_accepted": accepted})
}

func (s *server) playbackEvent(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in struct {
		Event            string  `json:"event"`
		PlaybackSession  string  `json:"playback_session_id"`
		SourceGeneration int     `json:"source_generation"`
		PositionSeconds  float64 `json:"position_seconds"`
		DurationSeconds  float64 `json:"duration_seconds"`
		Seekable         *bool   `json:"seekable"`
		PlaybackState    int     `json:"playback_state"`
		Details          string  `json:"details"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	event := strings.TrimSpace(in.Event)
	if event == "" {
		event = "PLAYER_EVENT"
	}
	if len(event) > 80 {
		event = event[:80]
	}
	details := strings.TrimSpace(in.Details)
	if len(details) > 600 {
		details = details[:600]
	}
	seekable := "unknown"
	if in.Seekable != nil {
		seekable = fmt.Sprint(*in.Seekable)
	}
	u := currentUser(r)
	s.admin.Log(r.Context(), "info", "playback", event, &u.ID, fmt.Sprintf(
		"media=%d session=%s generation=%d position=%.3f duration=%.3f seekable=%s state=%d details=%s",
		id, in.PlaybackSession, in.SourceGeneration, in.PositionSeconds, in.DurationSeconds, seekable, in.PlaybackState, details,
	))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) playbackStop(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	session := normalizePlaybackSessionID(r.URL.Query().Get("session"))
	cacheCleared := false
	if session != "" {
		switch {
		case streaming.IsSessionID(session):
			if manager, managerErr := streaming.ForDataDir(s.config.DataDir); managerErr == nil {
				cacheCleared = manager.Close(u.ID, session)
			}
		case transcode.IsSessionID(session):
			if manager, managerErr := transcode.ForDataDir(s.config.DataDir); managerErr == nil {
				cacheCleared = manager.Close(u.ID, session)
			}
		default:
			cacheCleared = s.hlsCache.CloseSession(u.ID, session)
		}
	}
	device := shortDevice(r.UserAgent())
	if err := s.admin.FinishPlayback(r.Context(), u.ID, id, device); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "hls_cache_cleared": cacheCleared})
}

func (s *server) monitoringOverview(w http.ResponseWriter, r *http.Request) {
	data, err := s.admin.MonitoringSafe(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	analytics, err := s.admin.MonitoringAnalytics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		admin.MonitoringOverview
		Analytics admin.MonitoringAnalytics `json:"analytics"`
	}{MonitoringOverview: data, Analytics: analytics})
}
