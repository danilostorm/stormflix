package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

func preferAACForPlayback(r *http.Request, plan *webcompat.Plan) {
	if plan == nil || !plan.Available || plan.AudioStream < 0 {
		return
	}
	// AAC forcing is an explicit compatibility fallback only. Do not infer it
	// merely from an Android User-Agent: the native player must first receive
	// the original multi-audio source and choose the profile-preferred track.
	// If Media3 later reports that preferred track as unsupported it asks for
	// audio=aac and only then do we encode that selected audio track.
	forceByQuery := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("audio")), "aac")
	if !forceByQuery {
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

func (s *server) compatibilityPlan(r *http.Request, id int64) (media.StreamItem, webcompat.Plan, error) {
	item, err := s.media.GetStreamItem(r.Context(), id)
	if err != nil {
		return item, webcompat.Plan{}, err
	}
	if !item.Available {
		return item, webcompat.Plan{}, sql.ErrNoRows
	}
	plan, err := webcompat.Probe(r.Context(), item.Path)
	if err != nil {
		return item, plan, err
	}
	preferAACForPlayback(r, &plan)
	return item, plan, nil
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
	item, plan, err := s.compatibilityPlan(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "mode": "unsupported", "reason": err.Error()})
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func compatibilityCacheKey(item media.StreamItem, plan webcompat.Plan) string {
	return fmt.Sprintf("media=%d;mtime=%d;size=%d;v=%d:%s;a=%d:%s;transcode=%t", item.ID, item.ModifiedUnix, item.SizeBytes, plan.VideoStream, plan.VideoCodec, plan.AudioStream, plan.AudioCodec, plan.AudioTranscode)
}

func (s *server) prepareRemuxMedia(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return
	}
	item, plan, err := s.compatibilityPlan(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	if !plan.Available {
		writeError(w, http.StatusUnprocessableEntity, errors.New(plan.Reason))
		return
	}
	cacheDir := filepath.Join(s.config.DataDir, "compat-cache")
	path, err := webcompat.MaterializeSeekable(r.Context(), item.Path, plan, cacheDir, compatibilityCacheKey(item, plan))
	if err != nil {
		uid := u.ID
		s.admin.Log(r.Context(), "error", "playback", "Compatibility materialization failed", &uid, err.Error())
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	stat, _ := os.Stat(path)
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":              true,
		"url":                fmt.Sprintf("/api/v1/media/%d/remux?audio=aac", id),
		"size_bytes":         size,
		"video_codec":        plan.VideoCodec,
		"audio_codec":        plan.AudioCodec,
		"source_audio_codec": plan.SourceAudioCodec,
		"audio_transcode":    plan.AudioTranscode,
		"seekable":           true,
	})
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
	item, plan, err := s.compatibilityPlan(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	if !plan.Available {
		writeError(w, http.StatusUnprocessableEntity, errors.New(plan.Reason))
		return
	}

	cacheDir := filepath.Join(s.config.DataDir, "compat-cache")
	compatPath, err := webcompat.MaterializeSeekable(r.Context(), item.Path, plan, cacheDir, compatibilityCacheKey(item, plan))
	if err != nil {
		uid := u.ID
		s.admin.Log(r.Context(), "error", "playback", "Compatibility materialization failed", &uid, err.Error())
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	file, err := os.Open(compatPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	mode := "web_remux"
	if plan.AudioTranscode {
		mode = "audio_aac"
	}
	_ = s.admin.Heartbeat(r.Context(), u.ID, id, shortDevice(r.UserAgent()), clientIP(r), admin.PlaybackHeartbeat{
		State:            "playing",
		Mode:             mode,
		VideoCodec:       plan.VideoCodec,
		AudioCodec:       plan.AudioCodec,
		SourceAudioCodec: plan.SourceAudioCodec,
	})
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=0")
	w.Header().Set("X-StormFlix-Playback", "direct-stream-remux")
	w.Header().Set("X-StormFlix-Seekable", "true")
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
	// ServeContent provides Content-Length and proper 206/Content-Range replies.
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}
