package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/danilostorm/stormflix/internal/playback"
	"github.com/danilostorm/stormflix/internal/subtitles"
)

// Native decoders probe the original themselves. This endpoint only reads local
// catalog/profile state: never stat/open/probe an rclone file before HTTP Range.
func (s *server) originalPlayback(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, ok := s.authorizeHLSMedia(w, r, id)
	if !ok {
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	resume := 0.0
	autoplay := true
	if profileID > 0 {
		err = s.db.QueryRowContext(r.Context(), `SELECT position_seconds FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, id).Scan(&resume)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if profile, e := s.auth.Profile(r.Context(), u.ID, profileID); e == nil {
			autoplay = profile.AutoplayNext
		}
	}
	prefs, _, err := s.loadPlaybackPreferences(r.Context(), profileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	markers, err := s.loadMediaMarkers(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// This query lists catalogued sidecars without opening the media or files.
	subtitleService := s.subtitles
	if subtitleService == nil {
		subtitleService = subtitles.NewService(s.db, s.config, nil)
	}
	sidecars, err := subtitleService.ListForMedia(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	container := strings.TrimPrefix(strings.ToLower(filepath.Ext(item.Path)), ".")
	plan := playback.Plan{Available: true, OriginalOnly: true, Mode: playback.ModeDirectPlay,
		MediaID: id, SourceContainer: container, Container: container, AudioStream: -1,
		ReasonCode: "native_original", Reason: "Arquivo original com decodificação no aparelho.",
		Transport: "original_range", Quality: "original", AvailableQualities: []string{"original"},
		ResumePositionSeconds: max(0, resume), PlaybackSessionID: newPlaybackSessionID(),
		URL: fmt.Sprintf("/api/v1/media/%d/stream", id)}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, struct {
		playback.Plan
		Preferences playbackPreferenceState `json:"playback_preferences"`
		Markers     []playbackMarkerState   `json:"markers"`
		Autoplay    bool                    `json:"autoplay_next"`
		Subtitles   []subtitles.Item        `json:"subtitles"`
	}{plan, prefs, markers, autoplay, sidecars})
}

// Retired native conversion URLs must not let stale clients start FFmpeg.
func (s *server) originalOnlyTransport(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":       "Reprodução por arquivo original. Atualize o player e tente novamente.",
		"reason_code": "server_conversion_disabled",
	})
}
