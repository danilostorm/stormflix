package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
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
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		items = s.filterRestrictedSeries(r, u.ID, items)
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
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		visible := s.filterRestrictedSeries(r, u.ID, []media.SeriesSummary{item.SeriesSummary})
		if len(visible) == 0 {
			writeError(w, http.StatusForbidden, errKidsRestricted)
			return
		}
	}
	writeJSON(w, http.StatusOK, item)
}
