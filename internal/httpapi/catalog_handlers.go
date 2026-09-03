package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"sync"

	"github.com/danilostorm/stormflix/internal/media"
)

func (s *server) homeFeed(w http.ResponseWriter, r *http.Request) {
	// Collection discovery and automatic marker analysis are lazy-started by the
	// first authenticated Home. They continue in background and Home never waits.
	s.startMovieCollectionIndexer()
	s.startMarkerAnalyzer(s.lifecycle)
	s.startCreditAnalyzer(s.lifecycle)

	u := currentUser(r)
	var allowed []int64
	if roleLevel(u.Role) < 2 {
		allowed = u.LibraryIDs
	}
	profileID := s.selectedProfileID(r, u.ID)

	// The Home used to do all of these independent reads sequentially. As the
	// catalog grows that makes latency additive. SQLite WAL supports concurrent
	// readers, so execute the independent rails together. The expensive static
	// grouped catalog is additionally cached for 20 seconds inside media.Service.
	var feed media.HomeFeed
	var feedErr error
	var continueItems, trendingNow, trendingWeek, releases []media.Item
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		feed, feedErr = s.media.HomeGroupedCached(r.Context(), allowed, s.config.HomeHeroMode, s.config.ServerName, s.config.ThemePreviewEnabled, s.config.ThemePreviewVolume, s.config.ThemePreviewAutoplay)
	}()
	go func() {
		defer wg.Done()
		if profileID > 0 {
			continueItems, _ = s.media.ContinueWatching(r.Context(), profileID, allowed, 24)
		}
	}()
	go func() { defer wg.Done(); trendingNow, _ = s.media.Trending(r.Context(), allowed, 2, 24) }()
	go func() { defer wg.Done(); trendingWeek, _ = s.media.Trending(r.Context(), allowed, 7, 24) }()
	go func() { defer wg.Done(); releases, _ = s.media.Releases(r.Context(), allowed, 24) }()
	wg.Wait()
	if feedErr != nil {
		writeError(w, http.StatusInternalServerError, feedErr)
		return
	}

	frontRows := []media.HomeRow{}
	if len(continueItems) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "continue-watching", Title: "Continuar assistindo", Items: continueItems})
	}
	if len(trendingNow) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "trending-now", Title: "Em alta agora", Items: trendingNow})
	}
	if len(trendingWeek) > 0 {
		frontRows = append(frontRows, media.HomeRow{ID: "trending-week", Title: "Em alta nesta semana", Items: trendingWeek})
	}
	if len(releases) > 0 {
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
		ContentRating    string  `json:"content_rating"`
		ContentRatingAge int     `json:"content_rating_age"`
		ReleaseDate      string  `json:"release_date"`
		PositionSeconds  float64 `json:"position_seconds"`
		DurationSeconds  float64 `json:"duration_seconds"`
		ProgressPercent  float64 `json:"progress_percent"`
		Completed        bool    `json:"completed"`
	}{Detail: detail, ContentRating: contentRating, ContentRatingAge: contentRatingAge, ReleaseDate: releaseDate}

	if profileID := s.selectedProfileID(r, u.ID); profileID > 0 {
		err = s.db.QueryRowContext(r.Context(), `SELECT position_seconds,duration_seconds,completed FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, id).
			Scan(&response.PositionSeconds, &response.DurationSeconds, &response.Completed)
		if err == nil && response.DurationSeconds > 0 {
			response.ProgressPercent = response.PositionSeconds / response.DurationSeconds * 100
		} else if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	writeJSON(w, http.StatusOK, response)
}
