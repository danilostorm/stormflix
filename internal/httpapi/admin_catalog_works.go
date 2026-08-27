package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// adminCatalogWork is the Plex-style catalog entity used by the Admin UI.
// Episodic libraries are represented by one logical show, never by one card per
// episode. Standalone libraries (movies/mixed/etc.) continue to expose items.
type adminCatalogWork struct {
	ID               int64  `json:"id"`
	EntityType       string `json:"entity_type"`
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
	ManualSeries     bool   `json:"manual_series"`
	SeriesKey        string `json:"series_key"`
	SeriesTitle      string `json:"series_title"`
	ContentRating    string `json:"content_rating"`
	ContentRatingAge int    `json:"content_rating_age"`
	ReleaseDate      string `json:"release_date"`
	SeasonCount      int    `json:"season_count"`
	EpisodeCount     int    `json:"episode_count"`
}

// adminCatalogWorks returns principal works for matching. For series/anime with
// seasons/cartoons this means one row per scanner-owned series_key. The old
// /admin/catalog endpoint remains available as a file-level diagnostic view.
func (s *server) adminCatalogWorks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	libraryID, _ := strconv.ParseInt(r.URL.Query().Get("library_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 160
	}

	items := make([]adminCatalogWork, 0, limit)

	seriesArgs := []any{}
	seriesQuery := `SELECT
MIN(m.id),m.library_id,l.name,
COALESCE(NULLIF(smo.title,''),MIN(NULLIF(si.series_title,'')),'Série'),
MIN(m.path),
CASE WHEN l.kind='anime_series' THEN 'anime' ELSE 'series' END,
COALESCE(MAX(mm.year),0),
CASE WHEN COALESCE(smo.provider,'')='tmdb' THEN COALESCE(smo.provider_id,0) ELSE COALESCE(MAX(mm.tmdb_id),0) END,
COALESCE((SELECT a.public_url FROM media_artwork a JOIN media_series_identity sx ON sx.media_id=a.media_id
  WHERE sx.library_id=m.library_id AND sx.series_key=si.series_key AND a.kind='poster' AND a.selected=1
  ORDER BY a.score DESC,a.id DESC LIMIT 1),''),
CASE
  WHEN COALESCE(smo.manual,0)=1 THEN 'matched'
  WHEN SUM(CASE WHEN COALESCE(mm.status,'pending')='matched' THEN 1 ELSE 0 END)>0 THEN 'matched'
  WHEN SUM(CASE WHEN COALESCE(mm.status,'pending')='error' THEN 1 ELSE 0 END)>0 THEN 'error'
  ELSE 'pending'
END,
CASE WHEN SUM(CASE WHEN COALESCE(mm.status,'pending')='error' THEN 1 ELSE 0 END)>0
  THEN CAST(SUM(CASE WHEN COALESCE(mm.status,'pending')='error' THEN 1 ELSE 0 END) AS TEXT)||' episódio(s) com erro'
  ELSE '' END,
0,
CASE WHEN COALESCE(smo.manual,0)=1 THEN 1 ELSE 0 END,
si.series_key,
COALESCE(NULLIF(smo.title,''),MIN(NULLIF(si.series_title,'')),'Série'),
COALESCE(MAX(NULLIF(mm.content_rating,'')),''),
COALESCE(MAX(mm.content_rating_age),-1),
COALESCE(MAX(NULLIF(mm.release_date,'')),''),
COUNT(DISTINCT CASE WHEN COALESCE(si.season_number,0)>0 THEN si.season_number ELSE 1 END),
COUNT(*)
FROM media_series_identity si
JOIN media m ON m.id=si.media_id
JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN series_metadata_overrides smo ON smo.library_id=si.library_id AND smo.series_key=si.series_key
WHERE m.available=1 AND si.series_key<>'' AND l.kind IN ('series','anime_series','animation_series')`
	if libraryID > 0 {
		seriesQuery += ` AND m.library_id=?`
		seriesArgs = append(seriesArgs, libraryID)
	}
	if q != "" {
		seriesQuery += ` AND (si.series_title LIKE ? OR COALESCE(smo.title,'') LIKE ? OR CAST(COALESCE(smo.provider_id,0) AS TEXT)=?)`
		seriesArgs = append(seriesArgs, "%"+q+"%", "%"+q+"%", q)
	}
	seriesQuery += ` GROUP BY m.library_id,l.name,l.kind,si.series_key,smo.provider,smo.provider_id,smo.title,smo.manual
ORDER BY CASE WHEN COALESCE(smo.manual,0)=1 THEN 0 ELSE 1 END,
COALESCE(NULLIF(smo.title,''),MIN(NULLIF(si.series_title,'')),'Série') COLLATE NOCASE
LIMIT ?`
	seriesArgs = append(seriesArgs, limit)

	rows, err := s.db.QueryContext(r.Context(), seriesQuery, seriesArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for rows.Next() {
		var item adminCatalogWork
		item.EntityType = "series"
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Path, &item.MediaType, &item.Year, &item.TMDBID,
			&item.PosterURL, &item.MetadataStatus, &item.LastError, &item.ManualMatch, &item.ManualSeries, &item.SeriesKey, &item.SeriesTitle,
			&item.ContentRating, &item.ContentRatingAge, &item.ReleaseDate, &item.SeasonCount, &item.EpisodeCount); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	remaining := limit - len(items)
	if remaining <= 0 {
		writeJSON(w, http.StatusOK, items)
		return
	}

	itemArgs := []any{}
	itemQuery := `SELECT m.id,m.library_id,l.name,m.title,m.path,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE(mm.status,'pending'),COALESCE(mm.last_error,''),COALESCE(mm.manual_match,0),
COALESCE(mm.content_rating,''),COALESCE(mm.content_rating_age,-1),COALESCE(mm.release_date,'')
FROM media m JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND l.kind<>'music' AND l.kind NOT IN ('series','anime_series','animation_series')`
	if libraryID > 0 {
		itemQuery += ` AND m.library_id=?`
		itemArgs = append(itemArgs, libraryID)
	}
	if q != "" {
		itemQuery += ` AND (m.title LIKE ? OR m.path LIKE ? OR CAST(COALESCE(mm.tmdb_id,0) AS TEXT)=?)`
		itemArgs = append(itemArgs, "%"+q+"%", "%"+q+"%", q)
	}
	itemQuery += ` ORDER BY COALESCE(mm.manual_match,0) DESC, CASE COALESCE(mm.status,'pending') WHEN 'error' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END, m.title COLLATE NOCASE LIMIT ?`
	itemArgs = append(itemArgs, remaining)

	rows, err = s.db.QueryContext(r.Context(), itemQuery, itemArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item adminCatalogWork
		item.EntityType = "item"
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Path, &item.MediaType, &item.Year, &item.TMDBID,
			&item.PosterURL, &item.MetadataStatus, &item.LastError, &item.ManualMatch, &item.ContentRating, &item.ContentRatingAge, &item.ReleaseDate); err != nil {
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
