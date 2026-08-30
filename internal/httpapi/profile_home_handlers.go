package httpapi

import (
	"context"
	"errors"
	"net/http"
)

type profileHomeMenuEntry struct {
	CategoryID int64 `json:"category_id"`
	Visible    bool  `json:"visible"`
	SortOrder  int   `json:"sort_order"`
}

func (s *server) selectedProfileHomeMenus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profileID := s.selectedProfileID(r, u.ID)
	if profileID <= 0 {
		writeJSON(w, http.StatusOK, []profileHomeMenuEntry{})
		return
	}
	writeJSON(w, http.StatusOK, s.profileHomeEntries(r.Context(), profileID))
}

func (s *server) profileHomeEntries(ctx context.Context, profileID int64) []profileHomeMenuEntry {
	rows, err := s.db.QueryContext(ctx, `SELECT category_id,visible,sort_order FROM profile_home_menus WHERE profile_id=? ORDER BY sort_order,category_id`, profileID)
	if err != nil {
		return []profileHomeMenuEntry{}
	}
	defer rows.Close()
	out := []profileHomeMenuEntry{}
	for rows.Next() {
		var v profileHomeMenuEntry
		if rows.Scan(&v.CategoryID, &v.Visible, &v.SortOrder) == nil {
			out = append(out, v)
		}
	}
	return out
}

func (s *server) adminProfileHomeOverview(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.name,p.is_kids,u.display_name FROM profiles p JOIN users u ON u.id=p.user_id WHERE p.active=1 ORDER BY u.display_name,p.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	profiles := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, user string
		var kids bool
		if err := rows.Scan(&id, &name, &kids, &user); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		profiles = append(profiles, map[string]any{"id": id, "name": name, "user": user, "is_kids": kids, "menus": s.profileHomeEntries(r.Context(), id)})
	}
	_ = rows.Close()
	categories, err := s.categories(r.Context(), true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	roots := []libraryCategory{}
	for _, c := range categories {
		if c.ParentID == nil {
			roots = append(roots, c)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles, "menus": roots})
}

func (s *server) updateProfileHomeMenus(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in struct {
		Menus []profileHomeMenuEntry `json:"menus"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM profiles WHERE id=?`, profileID).Scan(&exists); err != nil || exists == 0 {
		writeError(w, http.StatusNotFound, errors.New("profile not found"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM profile_home_menus WHERE profile_id=?`, profileID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	seen := map[int64]bool{}
	for i, menu := range in.Menus {
		if menu.CategoryID <= 0 || seen[menu.CategoryID] {
			continue
		}
		seen[menu.CategoryID] = true
		order := menu.SortOrder
		if order <= 0 {
			order = (i + 1) * 10
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO profile_home_menus(profile_id,category_id,visible,sort_order) SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM library_categories WHERE id=? AND parent_id IS NULL)`, profileID, menu.CategoryID, menu.Visible, order, menu.CategoryID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "profile", idString(profileID), "home", "Home personalizada do perfil atualizada", "", "", &uid)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
