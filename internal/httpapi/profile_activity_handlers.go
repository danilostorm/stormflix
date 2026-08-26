package httpapi

import (
	"net/http"
	"strconv"

	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) profileHistory(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if profileID <= 0 {
		writeJSON(w, http.StatusOK, []media.HistoryItem{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	items, err := s.media.ProfileHistory(r.Context(), profileID, allowed, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	restriction := s.selectedProfileRestriction(r, u.ID)
	if restriction.Restricted && len(items) > 0 {
		ids := make([]int64, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
		}
		ages, ageErr := s.ratingAges(r.Context(), ids)
		if ageErr != nil {
			writeError(w, http.StatusInternalServerError, ageErr)
			return
		}
		filtered := make([]media.HistoryItem, 0, len(items))
		for _, item := range items {
			age, ok := ages[item.ID]
			if !ok {
				age = -1
			}
			if ratingAllowed(age, restriction.Limit, item.Genres) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) profileStats(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	stats, err := s.media.ProfileStats(r.Context(), profileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *server) communityRanking(w http.ResponseWriter, r *http.Request) {
	items, err := s.media.League(r.Context(), 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":   "Liga StormFlix",
		"period":  "Este mês",
		"ranking": items,
	})
}
