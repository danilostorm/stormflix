package httpapi

import (
	"errors"
	"net/http"
	"strings"
)

// Jellyfin Android TV loads card artwork through a dedicated OkHttp image
// request that does not carry X-Emby-Token. The item UUID is still translated
// by jellyfinCompatWrap before this handler runs. Keep only the read-only image
// surface public; catalog, playback and progress APIs remain authenticated.
func (s *server) jellyfinPublicCatalogImage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	kind := strings.ToLower(strings.TrimSpace(r.PathValue("kind")))
	artKind := "poster"
	if strings.Contains(kind, "backdrop") {
		artKind = "backdrop"
	}

	url := ""
	if libID, ok := jfParsePrefixedID(id, "lib"); ok {
		var lib jellyfinLibrary
		if s.db.QueryRowContext(r.Context(), `SELECT id,name,kind FROM libraries WHERE id=? AND enabled=1`, libID).Scan(&lib.ID, &lib.Name, &lib.Kind) == nil {
			url = s.jellyfinLibraryArtworkURL(r.Context(), 0, lib)
		}
	} else if mediaID, ok := jfParsePrefixedID(id, "m"); ok {
		_ = s.db.QueryRowContext(r.Context(), `SELECT a.public_url
FROM media_artwork a JOIN media m ON m.id=a.media_id
WHERE m.id=? AND m.available=1 AND a.kind=? AND a.selected=1 AND a.public_url<>''
ORDER BY a.score DESC,a.id DESC LIMIT 1`, mediaID, artKind).Scan(&url)
	} else if seriesID, ok := jfParseSeriesID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, nil); err == nil {
			if artKind == "backdrop" {
				url = detail.BackdropURL
			} else {
				url = detail.PosterURL
			}
		}
	} else if seriesID, _, ok := jfParseSeasonID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, nil); err == nil {
			if artKind == "backdrop" {
				url = detail.BackdropURL
			} else {
				url = detail.PosterURL
			}
		}
	}

	if strings.TrimSpace(url) == "" {
		writeError(w, http.StatusNotFound, errors.New("image not found"))
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, url, http.StatusFound)
}
