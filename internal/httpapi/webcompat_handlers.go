package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

func preferAACForPlayback(r *http.Request, plan *webcompat.Plan) {
	if plan == nil || !plan.Available || plan.AudioStream < 0 {
		return
	}
	forceByQuery := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("audio")), "aac")
	forceByAndroid := strings.Contains(strings.ToLower(r.UserAgent()), "stormflix-android")
	if !forceByQuery && !forceByAndroid {
		return
	}

	sourceCodec := strings.ToLower(strings.TrimSpace(plan.SourceAudioCodec))
	if sourceCodec == "" {
		sourceCodec = strings.ToLower(strings.TrimSpace(plan.AudioCodec))
		plan.SourceAudioCodec = sourceCodec
	}
	if sourceCodec == "aac" {
		plan.AudioCodec = "aac"
		plan.AudioTranscode = false
		return
	}

	plan.AudioCodec = "aac"
	plan.AudioTranscode = true
	plan.Reason = "video will be copied without re-encoding; audio is forced to AAC for device compatibility"
}

func (s *server) mediaCompatibility(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
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
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	plan, err := webcompat.Probe(r.Context(), item.Path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"mode":      "unsupported",
			"reason":    err.Error(),
		})
		return
	}
	preferAACForPlayback(r, &plan)
	writeJSON(w, http.StatusOK, plan)
}

func (s *server) remuxMedia(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
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
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	plan, err := webcompat.Probe(r.Context(), item.Path)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	preferAACForPlayback(r, &plan)
	if !plan.Available {
		writeError(w, http.StatusUnprocessableEntity, errors.New(plan.Reason))
		return
	}

	s.admin.TouchPlayback(r.Context(), u.ID, id, shortDevice(r.UserAgent()), clientIP(r))
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-StormFlix-Playback", "direct-stream-remux")
	if plan.AudioTranscode {
		w.Header().Set("X-StormFlix-Transcoding", "audio-only")
		w.Header().Set("X-StormFlix-Audio-Policy", "aac-compatibility")
	} else {
		w.Header().Set("X-StormFlix-Transcoding", "false")
		w.Header().Set("X-StormFlix-Audio-Policy", "original")
	}
	w.Header().Set("X-StormFlix-Video-Codec", plan.VideoCodec)
	w.Header().Set("X-StormFlix-Audio-Codec", plan.AudioCodec)
	if plan.SourceAudioCodec != "" && plan.SourceAudioCodec != plan.AudioCodec {
		w.Header().Set("X-StormFlix-Source-Audio-Codec", plan.SourceAudioCodec)
	}
	if err := webcompat.Stream(r.Context(), item.Path, plan, w); err != nil {
		uid := u.ID
		s.admin.Log(r.Context(), "error", "playback", "Web remux failed", &uid, err.Error())
	}
}
