package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/media"
)

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
	var hb admin.PlaybackHeartbeat
	if decodeJSON(w, r, &hb) != nil {
		return
	}
	device := shortDevice(r.UserAgent())
	if err := s.admin.Heartbeat(r.Context(), u.ID, id, device, clientIP(r), hb); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) playbackStop(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	device := shortDevice(r.UserAgent())
	if err := s.admin.FinishPlayback(r.Context(), u.ID, id, device); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
