package httpapi

import (
	"net/http"
)

// jellyfinMe implements the canonical endpoint used by modern Jellyfin clients
// immediately after authentication. /Users/{id} alone is not enough because
// Android TV explicitly asks for /Users/Me while validating the saved session.
func (s *server) jellyfinMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.jellyfinUserObject(currentUser(r)))
}

// jellyfinDisplayPreferences returns a complete DisplayPreferencesDto. The
// Kotlin SDK marks most of this DTO as non-nullable, so returning 404 or a
// partial object during startup can abort the authenticated session.
func (s *server) jellyfinDisplayPreferences(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"Id":                 r.PathValue("id"),
		"ViewType":           "Poster",
		"SortBy":             "SortName",
		"IndexBy":            "None",
		"RememberIndexing":   false,
		"PrimaryImageHeight": 0,
		"PrimaryImageWidth":  0,
		"CustomPrefs":        map[string]string{},
		"ScrollDirection":    "Horizontal",
		"ShowBackdrop":       true,
		"RememberSorting":    false,
		"SortOrder":          "Ascending",
		"ShowSidebar":        true,
		"Client":             r.URL.Query().Get("client"),
	})
}

// Android TV reports its capabilities directly after login. StormFlix does
// not need to persist them yet, but acknowledging these endpoints is required
// for the client startup sequence to complete cleanly.
func (s *server) jellyfinCapabilities(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// These two home endpoints are requested by the default Android TV home
// sections. Returning empty, correctly-shaped responses is preferable to a
// protocol 404 while the rest of the compatibility catalog is loading.
func (s *server) jellyfinNextUp(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"Items":            []any{},
		"TotalRecordCount": 0,
		"StartIndex":       0,
	})
}

func (s *server) jellyfinLatest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}
