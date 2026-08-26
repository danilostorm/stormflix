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

	frontRows := []media.HomeRow{}
	profileID := s.selectedProfileID(r, u.ID)
	if profileID > 0 {
		items, progressErr := s.media.ContinueWatching(r.Context(), profileID, allowed, 24)
		if progressErr == nil && len(items) > 0 {
			frontRows = append(frontRows, media.HomeRow{ID: "continue-watching", Title: "Continuar assistindo", Items: items})
		}
	}
	if trending, trendErr := s.media.Trending(r.Context(), allowed, 2, 24); trendErr == nil && len(trending) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "trending-now", Title: "Em alta agora", Items: trending})
	}
	if weekly, trendErr := s.media.Trending(r.Context(), allowed, 7, 24); trendErr == nil && len(weekly) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "trending-week", Title: "Em alta nesta semana", Items: weekly})
	}
	if releases, releaseErr := s.media.Releases(r.Context(), allowed, 24); releaseErr == nil && len(releases) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "releases", Title: "Lançamentos", Items: releases})
	}
	feed.Rows = append(frontRows, feed.Rows...)

	if s.selectedProfileRestriction(r, u.ID).Restricted {
		s.filterRestrictedHome(r, u.ID, &feed)
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
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return
	}
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
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		detail.Related = s.filterRestrictedItems(r, u.ID, detail.Related)
	}
	if detail.Related == nil {
		detail.Related = []media.Item{}
	}
	var contentRating, releaseDate string
	contentRatingAge := -1
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(content_rating,''),COALESCE(content_rating_age,-1),COALESCE(release_date,'') FROM media_metadata WHERE media_id=?`, id).Scan(&contentRating, &contentRatingAge, &releaseDate)
	response := struct {
		media.Detail
		ContentRating    string `json:"content_rating"`
		ContentRatingAge int    `json:"content_rating_age"`
		ReleaseDate      string `json:"release_date"`
	}{Detail: detail, ContentRating: contentRating, ContentRatingAge: contentRatingAge, ReleaseDate: releaseDate}
	writeJSON(w, http.StatusOK, response)
}
