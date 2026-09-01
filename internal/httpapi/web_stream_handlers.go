package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/playback"
	"github.com/danilostorm/stormflix/internal/streaming"
)

func (s *server) playbackStreams(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, ok := s.authorizeHLSMedia(w, r, id)
	if !ok {
		return
	}
	source, err := s.probeMediaSource(r.Context(), id, item.Path, item.ModifiedUnix)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	audio := make([]playback.Stream, 0)
	subtitles := make([]playback.Stream, 0)
	for _, stream := range source.Streams {
		switch strings.ToLower(strings.TrimSpace(stream.Type)) {
		case "audio":
			audio = append(audio, stream)
		case "subtitle":
			subtitles = append(subtitles, stream)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"media_id":         id,
		"duration_seconds": source.DurationSeconds,
		"audio":            audio,
		"subtitles":        subtitles,
	})
}

func (s *server) webStreamPlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	sessionID := r.PathValue("session")
	if !streaming.IsSessionID(sessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid web playback session"))
		return
	}
	manager, err := streaming.ForDataDir(s.config.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	u := currentUser(r)
	playlist, err := manager.Playlist(u.ID, id, sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	playlist = rewritePlaylistWithPlaybackGrant(playlist, r.URL.Query().Get("st"))
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-StormFlix-Playback", "web-v53-continuous-session")
	_, _ = w.Write([]byte(playlist))
}

func (s *server) webStreamFragment(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	sessionID := r.PathValue("session")
	if !streaming.IsSessionID(sessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid web playback session"))
		return
	}
	name := strings.TrimSpace(r.PathValue("file"))
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		writeError(w, http.StatusBadRequest, errors.New("invalid web playback fragment"))
		return
	}
	manager, err := streaming.ForDataDir(s.config.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	u := currentUser(r)
	path, contentType, err := manager.FilePath(r.Context(), u.ID, id, sessionID, name)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("web playback fragment: %w", err))
		return
	}
	serveHLSFile(w, r, path, contentType)
}
