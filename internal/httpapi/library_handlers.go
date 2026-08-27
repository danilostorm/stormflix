package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/danilostorm/stormflix/internal/library"
	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) listLibraries(w http.ResponseWriter, r *http.Request) {
	items, err := s.libraries.List(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	u := currentUser(r)
	if roleLevel(u.Role) < 2 {
		allowed := map[int64]bool{}
		for _, id := range u.LibraryIDs {
			allowed[id] = true
		}
		out := make([]library.Library, 0, len(items))
		for _, v := range items {
			if allowed[v.ID] {
				out = append(out, v)
			}
		}
		items = out
	}
	if items == nil {
		items = []library.Library{}
	}
	writeJSON(w, 200, items)
}

func (s *server) createLibrary(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		Path    string   `json:"path"`
		Paths   []string `json:"paths"`
		Enabled bool     `json:"enabled"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	paths := in.Paths
	if len(paths) == 0 && strings.TrimSpace(in.Path) != "" {
		paths = []string{in.Path}
	}
	v, err := s.libraries.CreateMulti(r.Context(), in.Name, in.Kind, paths, in.Enabled)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "library", "Library created", &uid, strings.Join(v.Paths, " | "))
	writeJSON(w, 201, v)
}

func (s *server) updateLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var in struct {
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		Path    string   `json:"path"`
		Paths   []string `json:"paths"`
		Enabled bool     `json:"enabled"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	paths := in.Paths
	if len(paths) == 0 && strings.TrimSpace(in.Path) != "" {
		paths = []string{in.Path}
	}
	v, err := s.libraries.AdminUpdateMulti(r.Context(), id, in.Name, in.Kind, paths, in.Enabled)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "library", "Library updated", &uid, v.Name)
	writeJSON(w, 200, v)
}

func (s *server) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	v, err := s.libraries.ManagedGet(r.Context(), id)
	if err != nil {
		writeError(w, 404, errors.New("library not found"))
		return
	}
	if err := s.libraries.AdminDelete(r.Context(), id); err != nil {
		writeError(w, 500, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "library", "Library removed from catalog; files untouched", &uid, strings.Join(v.Paths, " | "))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	lib, job, err := s.libraries.EnqueueAdminScan(r.Context(), id)
	uid := currentUser(r).ID
	if err != nil {
		s.admin.Log(r.Context(), "error", "scanner", "Library scan could not be queued", &uid, err.Error())
		writeError(w, 400, err)
		return
	}
	s.admin.Log(r.Context(), "info", "scanner", "Library scan queued", &uid, lib.Name)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": job.Status, "job_id": job.ID, "library_id": id, "message": job.Message, "sources": lib.SourceCount, "online_sources": lib.OnlineSources})
}

func (s *server) cancelLibraryScan(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	uid := currentUser(r).ID
	if err := s.libraries.CancelQueuedOrRunningAdminScan(r.Context(), id); err != nil {
		s.admin.Log(r.Context(), "error", "scanner", "Library scan cancel failed", &uid, err.Error())
		writeError(w, 409, err)
		return
	}
	s.admin.Log(r.Context(), "info", "scanner", "Library scan cancel requested", &uid, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "cancelling", "library_id": id})
}

func (s *server) listMedia(w http.ResponseWriter, r *http.Request) {
	libraryID, _ := strconv.ParseInt(r.URL.Query().Get("library_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	items, err := s.media.List(r.Context(), libraryID, q, limit, offset, allowed)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		items = s.filterRestrictedItems(r, u.ID, items)
	}
	if items == nil {
		items = []media.Item{}
	}
	writeJSON(w, 200, items)
}

func (s *server) streamMedia(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, 404, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, 403, errors.New("library access denied"))
		return
	}

	// Keep Direct Play truly native. In particular, do not collapse a
	// multi-audio file into a server-selected track merely because the caller is
	// Android/Fire TV. Media3 must first see every source audio track so it can
	// apply the selected StormFlix profile language preference (pt-BR -> pt ->
	// por and Portuguese/Dublado/Brasil labels). The client explicitly requests
	// the compatibility/remux endpoint only when its decoder cannot play the
	// preferred track.
	file, err := os.Open(item.Path)
	if err != nil {
		writeError(w, 404, errors.New("media file is unavailable"))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	s.admin.TouchPlayback(r.Context(), u.ID, id, shortDevice(r.UserAgent()), clientIP(r))
	w.Header().Set("Content-Type", mediaContentType(item.Extension))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-StormFlix-Playback", "direct")
	w.Header().Set("Cache-Control", "private, max-age=0")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
}
