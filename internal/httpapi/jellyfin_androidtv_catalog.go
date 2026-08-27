package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func jellyfinQueryValue(r *http.Request, wanted string) string {
	for key, values := range r.URL.Query() {
		if !strings.EqualFold(key, wanted) || len(values) == 0 {
			continue
		}
		return strings.TrimSpace(values[0])
	}
	return ""
}

// jellyfinCatalogItems normalizes the camelCase query names emitted by the
// Kotlin SDK to the names the original compatibility handler expects. The
// anime_series kind is handled here as a TV-show collection without changing
// StormFlix's older Jellyfin handler surface.
func (s *server) jellyfinCatalogItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for _, key := range []string{"ParentId", "SearchTerm", "IncludeItemTypes"} {
		if query.Get(key) == "" {
			if value := jellyfinQueryValue(r, key); value != "" {
				query.Set(key, value)
			}
		}
	}
	r.URL.RawQuery = query.Encode()

	if libID, ok := jfParsePrefixedID(strings.TrimSpace(query.Get("ParentId")), "lib"); ok {
		var kind string
		if s.db.QueryRowContext(r.Context(), `SELECT kind FROM libraries WHERE id=? AND enabled=1`, libID).Scan(&kind) == nil && strings.EqualFold(kind, "anime_series") {
			series, err := s.media.SeriesList(r.Context(), []int64{libID}, strings.TrimSpace(query.Get("SearchTerm")))
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			items := make([]any, 0, len(series))
			for _, show := range series {
				items = append(items, s.jellyfinSeriesItem(show))
			}
			writeJSON(w, http.StatusOK, map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0})
			return
		}
	}
	s.jellyfinItems(w, r)
}

// jellyfinRichViews returns library folders as real Jellyfin CollectionFolder
// items. Android TV's UserViewCardPresenter only attempts to load artwork when
// a Primary image tag is present, otherwise it intentionally renders the blue
// folder placeholder.
func (s *server) jellyfinRichViews(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	libs, err := s.jellyfinLibraries(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]any, 0, len(libs))
	for _, lib := range libs {
		items = append(items, s.jellyfinLibraryItem(r.Context(), profileID, lib))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Items":            items,
		"TotalRecordCount": len(items),
		"StartIndex":       0,
	})
}

func (s *server) jellyfinLibraryItem(ctx context.Context, profileID int64, lib jellyfinLibrary) map[string]any {
	var childCount int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE library_id=? AND available=1`, lib.ID).Scan(&childCount)
	collectionType := jfCollectionType(lib.Kind)
	if strings.EqualFold(lib.Kind, "anime_series") {
		collectionType = "tvshows"
	}
	out := map[string]any{
		"Name":                    lib.Name,
		"ServerId":                s.jellyfinServerID(),
		"Id":                      jfLibraryID(lib.ID),
		"Type":                    "CollectionFolder",
		"CollectionType":          collectionType,
		"IsFolder":                true,
		"ChildCount":              childCount,
		"RecursiveItemCount":      childCount,
		"PrimaryImageAspectRatio": 16.0 / 9.0,
		"DateCreated":             time.Now().UTC().Format(time.RFC3339),
	}
	if s.jellyfinLibraryArtworkURL(ctx, profileID, lib) != "" {
		out["ImageTags"] = map[string]string{"Primary": "sf-library"}
	}
	return out
}

func (s *server) jellyfinLibraryArtworkURL(ctx context.Context, profileID int64, lib jellyfinLibrary) string {
	var artwork string
	_ = s.db.QueryRowContext(ctx, `SELECT a.public_url
FROM media_artwork a
JOIN media m ON m.id=a.media_id
WHERE m.library_id=? AND m.available=1 AND a.kind='poster' AND a.selected=1 AND a.public_url<>''
ORDER BY m.modified_unix DESC,a.score DESC,a.id DESC
LIMIT 1`, lib.ID).Scan(&artwork)
	if strings.TrimSpace(artwork) != "" {
		return artwork
	}
	if !strings.EqualFold(lib.Kind, "music") {
		return ""
	}
	tracks, err := s.music.Tracks(ctx, profileID, []int64{lib.ID}, "", 5000)
	if err != nil {
		return ""
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].ModifiedUnix > tracks[j].ModifiedUnix
	})
	for _, track := range tracks {
		if strings.TrimSpace(track.CoverURL) != "" {
			return track.CoverURL
		}
	}
	return ""
}

// jellyfinCatalogItem extends the normal item endpoint with CollectionFolder
// support. Android TV requests /Items/{libraryId} after opening a library.
func (s *server) jellyfinCatalogItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if libID, ok := jfParsePrefixedID(id, "lib"); ok {
		u := currentUser(r)
		profileID := s.jellyfinDefaultProfileID(r.Context(), u)
		libs, err := s.jellyfinLibraries(r.Context(), u)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, lib := range libs {
			if lib.ID == libID {
				writeJSON(w, http.StatusOK, s.jellyfinLibraryItem(r.Context(), profileID, lib))
				return
			}
		}
		writeError(w, http.StatusNotFound, errors.New("library not found"))
		return
	}
	s.jellyfinItem(w, r)
}

// jellyfinCatalogImage is retained for authenticated clients. Android TV's
// dedicated image loader does not attach the session token, so the registered
// route uses jellyfinPublicCatalogImage instead.
func (s *server) jellyfinCatalogImage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if libID, ok := jfParsePrefixedID(id, "lib"); ok {
		u := currentUser(r)
		profileID := s.jellyfinDefaultProfileID(r.Context(), u)
		libs, err := s.jellyfinLibraries(r.Context(), u)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, lib := range libs {
			if lib.ID != libID {
				continue
			}
			url := s.jellyfinLibraryArtworkURL(r.Context(), profileID, lib)
			if url == "" {
				writeError(w, http.StatusNotFound, errors.New("library image not found"))
				return
			}
			http.Redirect(w, r, url, http.StatusFound)
			return
		}
		writeError(w, http.StatusNotFound, errors.New("library not found"))
		return
	}
	s.jellyfinImage(w, r)
}

// jellyfinLatestCatalog returns actual recently-added content. Episodic anime
// libraries return recent episodes, while movie and music collections retain
// their native behavior.
func (s *server) jellyfinLatestCatalog(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.jellyfinDefaultProfileID(r.Context(), u)
	limit := 50
	if parsed, err := strconv.Atoi(jellyfinQueryValue(r, "limit")); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit > 100 {
		limit = 100
	}

	parent := jellyfinQueryValue(r, "parentId")
	libID, hasLibrary := jfParsePrefixedID(parent, "lib")
	if hasLibrary {
		var kind string
		if err := s.db.QueryRowContext(r.Context(), `SELECT kind FROM libraries WHERE id=? AND enabled=1`, libID).Scan(&kind); err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		if strings.EqualFold(kind, "music") {
			tracks, err := s.music.Tracks(r.Context(), profileID, []int64{libID}, "", 5000)
			if err != nil {
				writeJSON(w, http.StatusOK, []any{})
				return
			}
			sort.SliceStable(tracks, func(i, j int) bool {
				if tracks[i].ModifiedUnix == tracks[j].ModifiedUnix {
					return tracks[i].ID > tracks[j].ID
				}
				return tracks[i].ModifiedUnix > tracks[j].ModifiedUnix
			})
			if len(tracks) > limit {
				tracks = tracks[:limit]
			}
			items := make([]any, 0, len(tracks))
			for _, track := range tracks {
				items = append(items, s.jellyfinAudioItem(profileID, track))
			}
			writeJSON(w, http.StatusOK, items)
			return
		}
		if strings.EqualFold(kind, "anime_series") {
			episodes, err := s.media.RecentEpisodes(r.Context(), []int64{libID}, limit)
			if err != nil {
				writeJSON(w, http.StatusOK, []any{})
				return
			}
			items := make([]any, 0, len(episodes))
			for _, episode := range episodes {
				items = append(items, s.jellyfinMediaItem(profileID, episode))
			}
			writeJSON(w, http.StatusOK, items)
			return
		}
	}

	libraryFilter := int64(0)
	if hasLibrary {
		libraryFilter = libID
	}
	mediaItems, err := s.media.List(r.Context(), libraryFilter, "", 500, 0, u.LibraryIDs)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	sort.SliceStable(mediaItems, func(i, j int) bool {
		if mediaItems[i].ModifiedUnix == mediaItems[j].ModifiedUnix {
			return mediaItems[i].ID > mediaItems[j].ID
		}
		return mediaItems[i].ModifiedUnix > mediaItems[j].ModifiedUnix
	})
	if len(mediaItems) > limit {
		mediaItems = mediaItems[:limit]
	}
	items := make([]any, 0, len(mediaItems))
	for _, item := range mediaItems {
		items = append(items, s.jellyfinMediaItem(profileID, item))
	}
	writeJSON(w, http.StatusOK, items)
}
