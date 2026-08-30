package httpapi

import (
	"context"
	"net/http"
	"strconv"
)

func (s *server) recordCatalogChange(ctx context.Context, entityType, entityID, action, summary, beforeJSON, afterJSON string, userID *int64) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO catalog_changes(entity_type,entity_id,action,summary,before_json,after_json,user_id) VALUES(?,?,?,?,?,?,?)`, entityType, entityID, action, summary, beforeJSON, afterJSON, userID)
}

func idString(id int64) string { return strconv.FormatInt(id, 10) }

func (s *server) catalogHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id,c.entity_type,c.entity_id,c.action,c.summary,c.before_json,c.after_json,COALESCE(u.display_name,''),c.created_at FROM catalog_changes c LEFT JOIN users u ON u.id=c.user_id ORDER BY c.id DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var entityType, entityID, action, summary, beforeJSON, afterJSON, user, created string
		if err := rows.Scan(&id, &entityType, &entityID, &action, &summary, &beforeJSON, &afterJSON, &user, &created); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, map[string]any{"id": id, "entity_type": entityType, "entity_id": entityID, "action": action, "summary": summary, "before": beforeJSON, "after": afterJSON, "user": user, "created_at": created})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) previewLibraryScan(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	preview, err := s.libraries.PreviewMulti(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *server) requireAutomaticBackup(w http.ResponseWriter, r *http.Request, note string) bool {
	if _, err := s.ensureAutomaticBackup(r.Context(), note); err != nil {
		uid := currentUser(r).ID
		s.admin.Log(r.Context(), "error", "backup", "Safety backup failed; protected operation aborted", &uid, err.Error())
		writeError(w, http.StatusInternalServerError, err)
		return false
	}
	return true
}

func (s *server) scanLibraryWithBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAutomaticBackup(w, r, "antes do scan da biblioteca") {
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "library", r.PathValue("id"), "scan", "Scan de biblioteca solicitado", "", "", &uid)
	s.scanLibrary(w, r)
}

func (s *server) scanAllLibrariesWithBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAutomaticBackup(w, r, "antes do scan de todas as bibliotecas") {
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "library", "all", "scan_all", "Scan de todas as bibliotecas solicitado", "", "", &uid)
	s.scanAllLibraries(w, r)
}

func (s *server) updateLibraryWithBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAutomaticBackup(w, r, "antes de alterar biblioteca/caminho") {
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "library", r.PathValue("id"), "update", "Biblioteca/caminho alterado", "", "", &uid)
	s.updateLibrary(w, r)
}

func (s *server) organizeRecommendedCategoriesWithBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAutomaticBackup(w, r, "antes de reorganizar os menus da Home") {
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "category", "all", "organize", "Estrutura recomendada da Home solicitada", "", "", &uid)
	s.organizeRecommendedCategories(w, r)
}
