package httpapi

import (
	"net/http"
	"strings"
)

func (s *server) personTitles(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, errBadRequest("person name is required"))
		return
	}
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	result, err := s.media.PersonTitles(r.Context(), name, allowed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.selectedProfileIsKids(r, u.ID) {
		result.Items = filterKidsItems(result.Items)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) continueWatching(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if profileID <= 0 {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	items, err := s.media.ContinueWatching(r.Context(), profileID, allowed, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if s.selectedProfileIsKids(r, u.ID) {
		items = filterKidsItems(items)
	}
	writeJSON(w, http.StatusOK, items)
}

type simpleBadRequest string

func (e simpleBadRequest) Error() string { return string(e) }
func errBadRequest(message string) error { return simpleBadRequest(message) }
