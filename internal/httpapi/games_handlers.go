package httpapi

import (
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danilostorm/stormflix/internal/games"
)

func (s *server) gameAllowedLibraries(r *http.Request) []int64 {
	u := currentUser(r)
	if roleLevel(u.Role) >= 2 {
		return nil
	}
	return u.LibraryIDs
}

func (s *server) gamesHome(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	home, err := s.games.Home(r.Context(), profileID, s.gameAllowedLibraries(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, home)
}

func (s *server) gamesList(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	favorites := strings.EqualFold(r.URL.Query().Get("favorites"), "true") || r.URL.Query().Get("favorites") == "1"
	items, err := s.games.List(r.Context(), profileID, s.gameAllowedLibraries(r), r.URL.Query().Get("q"), r.URL.Query().Get("platform"), favorites, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) gameDetail(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	game, err := s.games.PlayDetail(r.Context(), id, s.selectedProfileID(r, u.ID), s.gameAllowedLibraries(r))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (s *server) gameFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if _, err := s.games.PlayDetail(r.Context(), id, profileID, s.gameAllowedLibraries(r)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("game not found"))
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	var in struct {
		Favorite bool `json:"favorite"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if err := s.games.SetFavorite(r.Context(), id, profileID, in.Favorite); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "favorite": in.Favorite})
}

func (s *server) gameCover(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if _, err := s.games.PlayDetail(r.Context(), id, s.selectedProfileID(r, u.ID), s.gameAllowedLibraries(r)); err != nil {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	path, err := s.games.CoverPath(r.Context(), id)
	if err != nil || strings.TrimSpace(path) == "" {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > 20<<20 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

func (s *server) gameROM(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	file, err := s.games.PlayableFile(r.Context(), id, profileID, s.gameAllowedLibraries(r))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	handle, err := os.Open(file.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("ROM file is unavailable"))
		return
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("ROM file is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": file.Name}))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-StormFlix-ROM-Name", file.Name)
	http.ServeContent(w, r, file.Name, info.ModTime(), handle)
}

func (s *server) gameSaveStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	game, err := s.games.PlayDetail(r.Context(), id, s.selectedProfileID(r, u.ID), s.gameAllowedLibraries(r))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	writeJSON(w, http.StatusOK, game.Saves)
}

func (s *server) gameSaveRead(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.PathValue("kind")))
	if games.MaxSaveBytes(kind) <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid game save kind"))
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if _, err := s.games.PlayDetail(r.Context(), id, profileID, s.gameAllowedLibraries(r)); err != nil {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	file, err := s.games.SaveFile(r.Context(), profileID, id, kind, 0)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("game save not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	handle, err := os.Open(file.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("game save not found"))
		return
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("game save not found"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, filepath.Base(file.Path), info.ModTime(), handle)
}

func (s *server) gameSaveWrite(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.PathValue("kind")))
	max := games.MaxSaveBytes(kind)
	if max <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("invalid game save kind"))
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if _, err := s.games.PlayDetail(r.Context(), id, profileID, s.gameAllowedLibraries(r)); err != nil {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("game save payload is too large"))
		return
	}
	info, err := s.games.WriteSave(r.Context(), profileID, id, kind, 0, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *server) gamePlaybackHeartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if _, err := s.games.PlayDetail(r.Context(), id, profileID, s.gameAllowedLibraries(r)); err != nil {
		writeError(w, http.StatusNotFound, errors.New("game not found"))
		return
	}
	var in struct {
		SessionID      string `json:"session_id"`
		ElapsedSeconds int64  `json:"elapsed_seconds"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	total, err := s.games.Heartbeat(r.Context(), profileID, id, in.SessionID, in.ElapsedSeconds)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "play_seconds": total})
}

func (s *server) gameRuntimeNostalgist(w http.ResponseWriter, r *http.Request) {
	s.serveGameRuntime(w, r, "nostalgist.js")
}

func (s *server) gameRuntimeCore(w http.ResponseWriter, r *http.Request) {
	s.serveGameRuntime(w, r, r.PathValue("asset"))
}

func (s *server) serveGameRuntime(w http.ResponseWriter, r *http.Request, asset string) {
	path, contentType, err := s.games.RuntimeAsset(r.Context(), asset)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusBadGateway, errors.New("game runtime cache is unavailable"))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-StormFlix-Game-Runtime", "pinned")
	http.ServeFile(w, r, path)
}

func (s *server) scanLibraryDispatchWithBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAutomaticBackup(w, r, "antes do scan da biblioteca", false) {
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "library", r.PathValue("id"), "scan", "Scan de biblioteca solicitado", "", "", &uid)
	s.scanLibraryDispatch(w, r)
}

func (s *server) scanLibraryDispatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	isGames, err := s.games.IsGamesLibrary(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("library not found"))
		return
	}
	if !isGames {
		s.scanLibrary(w, r)
		return
	}
	job, err := s.games.EnqueueScan(r.Context(), id)
	uid := currentUser(r).ID
	if err != nil {
		s.admin.Log(r.Context(), "error", "games", "Game library scan could not be queued", &uid, err.Error())
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.admin.Log(r.Context(), "info", "games", "Game library scan queued", &uid, job.Library)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": job.Status, "job_id": job.ID, "library_id": id, "message": job.Message, "kind": "game_scan"})
}

func (s *server) cancelLibraryScanDispatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	isGames, err := s.games.IsGamesLibrary(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("library not found"))
		return
	}
	if !isGames {
		s.cancelLibraryScan(w, r)
		return
	}
	uid := currentUser(r).ID
	if err := s.games.CancelScan(r.Context(), id); err != nil {
		s.admin.Log(r.Context(), "error", "games", "Game scan cancel failed", &uid, err.Error())
		writeError(w, http.StatusConflict, err)
		return
	}
	s.admin.Log(r.Context(), "info", "games", "Game scan cancel requested", &uid, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "cancelling", "library_id": id})
}
