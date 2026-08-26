package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
)

var srtTimeRE = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}),(\d{3})`)

func (s *server) mediaVersions(w http.ResponseWriter, r *http.Request) {
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
	items, err := s.media.Versions(r.Context(), id, allowed)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) subtitleVTT(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	subtitleID, err := parseID(r.PathValue("subtitle_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, mediaID) {
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), mediaID)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}
	var assetPath, format string
	err = s.db.QueryRowContext(r.Context(), `SELECT asset_path,format FROM subtitles WHERE id=? AND media_id=?`, subtitleID, mediaID).Scan(&assetPath, &format)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("subtitle not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	path, err := s.assets.Resolve(assetPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	if strings.EqualFold(format, "vtt") || strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
		if !strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
			text = "WEBVTT\n\n" + text
		}
	} else if strings.EqualFold(format, "srt") {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		text = srtTimeRE.ReplaceAllString(text, `$1.$2`)
		text = "WEBVTT\n\n" + text
	} else {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("subtitle format cannot be rendered by the web player"))
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write([]byte(text))
}
