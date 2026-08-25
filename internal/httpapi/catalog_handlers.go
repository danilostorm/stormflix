package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) homeFeed(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	feed, err := s.media.HomeGrouped(r.Context(), allowed, s.config.HomeHeroMode, s.config.ServerName, s.config.ThemePreviewEnabled, s.config.ThemePreviewVolume, s.config.ThemePreviewAutoplay)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

func (s *server) mediaDetails(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	detail, err := s.media.Detail(r.Context(), id, allowed)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		if err.Error() == "library access denied" {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if detail.Related == nil {
		detail.Related = []media.Item{}
	}
	writeJSON(w, http.StatusOK, detail)
}
