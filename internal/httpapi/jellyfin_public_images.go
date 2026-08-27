package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"os"
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

	artworkURL := ""
	if libID, ok := jfParsePrefixedID(id, "lib"); ok {
		var lib jellyfinLibrary
		if s.db.QueryRowContext(r.Context(), `SELECT id,name,kind FROM libraries WHERE id=? AND enabled=1`, libID).Scan(&lib.ID, &lib.Name, &lib.Kind) == nil {
			artworkURL = s.jellyfinLibraryArtworkURL(r.Context(), 0, lib)
		}
	} else if mediaID, ok := jfParsePrefixedID(id, "m"); ok {
		_ = s.db.QueryRowContext(r.Context(), `SELECT a.public_url
FROM media_artwork a JOIN media m ON m.id=a.media_id
WHERE m.id=? AND m.available=1 AND a.kind=? AND a.selected=1 AND a.public_url<>''
ORDER BY a.score DESC,a.id DESC LIMIT 1`, mediaID, artKind).Scan(&artworkURL)
	} else if seriesID, ok := jfParseSeriesID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, nil); err == nil {
			if artKind == "backdrop" {
				artworkURL = detail.BackdropURL
			} else {
				artworkURL = detail.PosterURL
			}
		}
	} else if seriesID, _, ok := jfParseSeasonID(id); ok {
		if detail, err := s.media.SeriesDetail(r.Context(), seriesID, nil); err == nil {
			if artKind == "backdrop" {
				artworkURL = detail.BackdropURL
			} else {
				artworkURL = detail.PosterURL
			}
		}
	}

	if strings.TrimSpace(artworkURL) == "" {
		writeError(w, http.StatusNotFound, errors.New("image not found"))
		return
	}
	serveJellyfinArtworkURL(s, w, r, artworkURL)
}

// serveJellyfinArtworkURL avoids redirecting official Jellyfin clients to the
// native /assets route. That route intentionally requires a StormFlix session,
// while Jellyfin Android TV's Coil/OkHttp image loader does not attach the
// StormFlix cookie or X-Emby-Token to artwork requests. If the selected artwork
// is stored in StormFlix's local asset store, serve the file directly from this
// already-public, read-only Jellyfin image endpoint. External CDN/source URLs
// still use a normal redirect.
func serveJellyfinArtworkURL(s *server, w http.ResponseWriter, r *http.Request, rawURL string) {
	target := strings.TrimSpace(rawURL)
	assetKey := ""

	if strings.HasPrefix(target, "/assets/") {
		assetKey = strings.TrimPrefix(target, "/assets/")
	} else if parsed, err := url.Parse(target); err == nil && strings.HasPrefix(parsed.Path, "/assets/") {
		assetKey = strings.TrimPrefix(parsed.Path, "/assets/")
	}

	if assetKey != "" {
		if path, err := s.assets.Resolve(assetKey); err == nil {
			if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
				w.Header().Set("Cache-Control", "public, max-age=86400")
				http.ServeFile(w, r, path)
				return
			}
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, target, http.StatusFound)
}
