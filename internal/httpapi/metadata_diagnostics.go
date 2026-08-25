package httpapi

import (
	"net/http"
	"strconv"
)

type metadataErrorItem struct {
	MediaID   int64  `json:"media_id"`
	Title     string `json:"title"`
	Library   string `json:"library"`
	Path      string `json:"path"`
	LastError string `json:"last_error"`
	UpdatedAt string `json:"updated_at"`
}

func (s *server) metadataErrors(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT m.id,m.title,l.name,m.path,mm.last_error,mm.updated_at
FROM media_metadata mm
JOIN media m ON m.id=mm.media_id
JOIN libraries l ON l.id=m.library_id
WHERE mm.status='error'
ORDER BY mm.updated_at DESC,m.id DESC
LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []metadataErrorItem{}
	for rows.Next() {
		var item metadataErrorItem
		if err := rows.Scan(&item.MediaID, &item.Title, &item.Library, &item.Path, &item.LastError, &item.UpdatedAt); err != nil {
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

func (s *server) testTMDBAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.metadata.TestTMDB(r.Context()); err != nil {
		uid := currentUser(r).ID
		s.admin.Log(r.Context(), "error", "metadata", "TMDB connectivity test failed", &uid, err.Error())
		writeError(w, http.StatusBadGateway, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "metadata", "TMDB connectivity test succeeded", &uid, "TMDB API authentication and connectivity are working")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "TMDB conectado e autenticado com sucesso."})
}
