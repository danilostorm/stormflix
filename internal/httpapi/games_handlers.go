package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	game, err := s.games.Detail(r.Context(), id, s.selectedProfileID(r, u.ID), s.gameAllowedLibraries(r))
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
	if _, err := s.games.Detail(r.Context(), id, profileID, s.gameAllowedLibraries(r)); err != nil {
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
	if _, err := s.games.Detail(r.Context(), id, s.selectedProfileID(r, u.ID), s.gameAllowedLibraries(r)); err != nil {
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
