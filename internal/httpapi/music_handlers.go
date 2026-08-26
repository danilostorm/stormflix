package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) musicHome(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	home, err := s.music.Home(r.Context(), profileID, allowed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, home)
}

func (s *server) musicTracks(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tracks, err := s.music.Tracks(r.Context(), profileID, allowed, strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *server) musicTrack(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	track, err := s.music.Track(r.Context(), profileID, id, allowed)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("track not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *server) musicStream(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	if _, err := s.music.Track(r.Context(), s.selectedProfileID(r, u.ID), id, allowed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("track not found"))
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("track not found"))
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
	file, err := os.Open(item.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("audio file is unavailable"))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", musicContentType(item.Extension))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-StormFlix-Playback", "direct")
	w.Header().Set("Cache-Control", "private, max-age=0")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}

func (s *server) musicListening(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if profileID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("select a profile first"))
		return
	}
	var in struct {
		DeltaSeconds float64 `json:"delta_seconds"`
		Started      bool    `json:"started"`
		Completed    bool    `json:"completed"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if err := s.music.RecordListening(r.Context(), profileID, id, in.DeltaSeconds, in.Started, in.Completed); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) musicFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	favorite, err := s.music.ToggleFavorite(r.Context(), profileID, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": favorite})
}

func (s *server) musicLyrics(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	if _, err := s.music.Track(r.Context(), s.selectedProfileID(r, u.ID), id, allowed); err != nil {
		writeError(w, http.StatusNotFound, errors.New("track not found"))
		return
	}
	lyrics, err := s.music.Lyrics(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"provider": "lrclib", "plain_lyrics": "", "synced_lyrics": "", "instrumental": false, "found": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, lyrics)
}

func (s *server) adminMusicIndex(w http.ResponseWriter, r *http.Request) {
	started := s.music.StartIndexing()
	uid := currentUser(r).ID
	if started {
		s.admin.Log(r.Context(), "info", "music", "Music metadata indexing started", &uid, "FFprobe + MusicBrainz + Cover Art Archive")
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"started": started, "indexing": true})
}

func musicContentType(extension string) string {
	switch strings.ToLower(extension) {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".wma":
		return "audio/x-ms-wma"
	default:
		return "application/octet-stream"
	}
}
