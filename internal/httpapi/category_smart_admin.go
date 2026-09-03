package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
)

func (s *server) previewSmartCategory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CategoryID int64         `json:"category_id"`
		ParentID   int64         `json:"parent_id"`
		RuleMode   string        `json:"rule_mode"`
		LibraryIDs []int64       `json:"library_ids"`
		Rules      categoryRules `json:"rules"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.RuleMode = normalizeRuleMode(in.RuleMode)
	in.Rules = normalizeRules(in.Rules)
	ids := append([]int64(nil), in.LibraryIDs...)
	if (in.RuleMode == "rules" || len(ids) == 0) && in.ParentID > 0 {
		ids, _ = s.categoryTreeLibraries(r.Context(), in.ParentID)
	}
	if roleLevel(currentUser(r).Role) < 2 {
		ids = intersectIDs(ids, currentUser(r).LibraryIDs)
	}
	items, err := s.media.CatalogList(r.Context(), 0, "", 500, 0, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	series, err := s.media.SeriesList(r.Context(), ids, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	count := 0
	pending := false
	samples := []map[string]any{}
	seen := map[string]bool{}
	add := func(key, title, poster string, id int64) {
		if seen[key] {
			return
		}
		seen[key] = true
		count++
		if len(samples) < 10 {
			samples = append(samples, map[string]any{"id": id, "title": title, "poster_url": poster})
		}
	}
	for _, item := range series {
		if s.smartItemMatches(r.Context(), item.RepresentativeMediaID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, "matched", in.Rules, in.RuleMode, &pending) {
			add("s:"+item.ID, item.Title, item.PosterURL, item.RepresentativeMediaID)
		}
	}
	for _, item := range items {
		if item.EntityType == "series" || item.EpisodeNumber > 0 || item.MediaType == "series" {
			continue
		}
		if s.smartItemMatches(r.Context(), item.ID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, item.MetadataStatus, in.Rules, in.RuleMode, &pending) {
			add("m:"+idString(item.ID), item.Title, item.PosterURL, item.ID)
		}
	}
	if pending {
		s.kickTechnicalIndexer()
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count, "samples": samples, "technical_pending": pending})
}

func (s *server) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ParentID *int64  `json:"parent_id"`
		IDs      []int64 `json:"ids"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if len(in.IDs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("category order is empty"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	for i, id := range in.IDs {
		var res sql.Result
		if in.ParentID == nil || *in.ParentID == 0 {
			res, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND parent_id IS NULL`, (i+1)*10, id)
		} else {
			res, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND parent_id=?`, (i+1)*10, id, *in.ParentID)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid category in order"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "category", "order", "reorder", "Ordem de menus/seções alterada", "", "", &uid)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
