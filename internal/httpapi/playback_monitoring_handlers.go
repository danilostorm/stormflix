package httpapi

import (
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) playbackHeartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil { writeError(w, http.StatusBadRequest, err); return }
	item, err := s.media.GetStreamItem(r.Context(), id)
	if err != nil || !item.Available { writeError(w, http.StatusNotFound, err); return }
	u := currentUser(r)
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) { writeError(w, http.StatusForbidden, errLibraryDenied); return }
	var hb admin.PlaybackHeartbeat
	if decodeJSON(w, r, &hb) != nil { return }
	device := shortDevice(r.UserAgent())
	if err := s.admin.Heartbeat(r.Context(), u.ID, id, device, clientIP(r), hb); err != nil { writeError(w, http.StatusInternalServerError, err); return }
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) playbackStop(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil { writeError(w, http.StatusBadRequest, err); return }
	u := currentUser(r)
	device := shortDevice(r.UserAgent())
	if err := s.admin.FinishPlayback(r.Context(), u.ID, id, device); err != nil { writeError(w, http.StatusInternalServerError, err); return }
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) monitoringOverview(w http.ResponseWriter, r *http.Request) {
	data, err := s.admin.Monitoring(r.Context())
	if err != nil { writeError(w, http.StatusInternalServerError, err); return }
	writeJSON(w, http.StatusOK, data)
}

var errLibraryDenied = &accessError{"library access denied"}

type accessError struct{ message string }
func (e *accessError) Error() string { return strings.TrimSpace(e.message) }
