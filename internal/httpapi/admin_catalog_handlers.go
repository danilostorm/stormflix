package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type adminCatalogItem struct {
	ID               int64  `json:"id"`
	LibraryID        int64  `json:"library_id"`
	LibraryName      string `json:"library_name"`
	Title            string `json:"title"`
	Path             string `json:"path"`
	MediaType        string `json:"media_type"`
	Year             int    `json:"year"`
	TMDBID           int64  `json:"tmdb_id"`
	PosterURL        string `json:"poster_url"`
	MetadataStatus   string `json:"metadata_status"`
	LastError        string `json:"last_error"`
	ManualMatch      bool   `json:"manual_match"`
	ContentRating    string `json:"content_rating"`
	ContentRatingAge int    `json:"content_rating_age"`
	ReleaseDate      string `json:"release_date"`
}

func (s *server) adminCatalog(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	libraryID, _ := strconv.ParseInt(r.URL.Query().Get("library_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 120
	}
	args := []any{}
	query := `SELECT m.id,m.library_id,l.name,m.title,m.path,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE(mm.status,'pending'),COALESCE(mm.last_error,''),COALESCE(mm.manual_match,0),
COALESCE(mm.content_rating,''),COALESCE(mm.content_rating_age,-1),COALESCE(mm.release_date,'')
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND l.kind<>'music'`
	if libraryID > 0 {
		query += ` AND m.library_id=?`
		args = append(args, libraryID)
	}
	if q != "" {
		query += ` AND (m.title LIKE ? OR m.path LIKE ? OR CAST(COALESCE(mm.tmdb_id,0) AS TEXT)=?)`
		args = append(args, "%"+q+"%", "%"+q+"%", q)
	}
	query += ` ORDER BY COALESCE(mm.manual_match,0) DESC, CASE COALESCE(mm.status,'pending') WHEN 'error' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END, m.title COLLATE NOCASE LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []adminCatalogItem{}
	for rows.Next() {
		var item adminCatalogItem
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Path, &item.MediaType, &item.Year, &item.TMDBID, &item.PosterURL, &item.MetadataStatus, &item.LastError, &item.ManualMatch, &item.ContentRating, &item.ContentRatingAge, &item.ReleaseDate); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) adminCatalogMatches(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := s.metadata.SearchForMedia(r.Context(), id, strings.TrimSpace(r.URL.Query().Get("q")))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) adminCatalogMatch(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in struct {
		TMDBID      int64  `json:"tmdb_id"`
		MediaType   string `json:"media_type"`
		ApplyCopies bool   `json:"apply_copies"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	updated, err := s.metadata.ManualMatch(r.Context(), id, in.TMDBID, strings.ToLower(strings.TrimSpace(in.MediaType)), in.ApplyCopies)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "catalog", "Correspondência manual aplicada", &uid, strconv.FormatInt(id, 10)+" · TMDB "+strconv.FormatInt(in.TMDBID, 10))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": updated})
}

func (s *server) adminCatalogAuto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.metadata.ResetManualMatch(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("media not found"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "catalog", "Correspondência voltou ao modo automático", &uid, strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
