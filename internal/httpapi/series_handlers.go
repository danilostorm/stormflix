package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *server) listSeries(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	items, err := s.media.SeriesList(r.Context(), allowed, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) seriesDetails(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	item, err := s.media.SeriesDetail(r.Context(), strings.TrimSpace(r.PathValue("id")), allowed)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("series not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
