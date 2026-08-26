package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
)

type libraryCategory struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Slug       string  `json:"slug"`
	Kind       string  `json:"kind"`
	SortOrder  int     `json:"sort_order"`
	Active     bool    `json:"active"`
	System     bool    `json:"system"`
	LibraryIDs []int64 `json:"library_ids"`
}

var categorySlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func (s *server) listCategories(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	items, err := s.categories(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if roleLevel(u.Role) < 2 {
		for i := range items {
			items[i].LibraryIDs = intersectIDs(items[i].LibraryIDs, u.LibraryIDs)
		}
	}
	visible := items[:0]
	for _, c := range items {
		if c.Active && len(c.LibraryIDs) > 0 {
			visible = append(visible, c)
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (s *server) adminCategories(w http.ResponseWriter, r *http.Request) {
	items, err := s.categories(r.Context(), true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) browseCategory(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	var c libraryCategory
	err := s.db.QueryRowContext(r.Context(), `SELECT id,name,slug,kind,sort_order,active,system FROM library_categories WHERE slug=? AND active=1`, slug).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Kind, &c.SortOrder, &c.Active, &c.System)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("category not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ids, err := s.categoryLibraries(r.Context(), c.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	u := currentUser(r)
	if roleLevel(u.Role) < 2 {
		ids = intersectIDs(ids, u.LibraryIDs)
	}
	if ids == nil {
		ids = []int64{}
	}
	response := struct {
		Category libraryCategory       `json:"category"`
		Media    []media.Item          `json:"media"`
		Series   []media.SeriesSummary `json:"series"`
	}{Category: c, Media: []media.Item{}, Series: []media.SeriesSummary{}}
	response.Category.LibraryIDs = ids
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, response)
		return
	}
	if c.Kind == "series" || c.Kind == "anime" || c.Kind == "mixed" {
		response.Series, err = s.media.SeriesList(r.Context(), ids, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if c.Kind != "series" {
		items, listErr := s.media.List(r.Context(), 0, "", 500, 0, ids)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, listErr)
			return
		}
		for _, item := range items {
			if item.EpisodeNumber > 0 || item.MediaType == "series" {
				continue
			}
			switch c.Kind {
			case "movie":
				if item.MediaType != "movie" {
					continue
				}
			case "anime":
				if item.MediaType != "anime" {
					continue
				}
			}
			response.Media = append(response.Media, item)
		}
	}
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		response.Media = s.filterRestrictedItems(r, u.ID, response.Media)
		response.Series = s.filterRestrictedSeries(r, u.ID, response.Series)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) createCategory(w http.ResponseWriter, r *http.Request) {
	var in libraryCategory
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Kind = normalizeCategoryKind(in.Kind)
	if in.Name == "" || !categorySlugRE.MatchString(in.Slug) {
		writeError(w, http.StatusBadRequest, errors.New("name and a valid slug are required"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO library_categories(name,slug,kind,sort_order,active,system) VALUES(?,?,?,?,?,0)`, in.Name, in.Slug, in.Kind, in.SortOrder, in.Active)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	id, _ := res.LastInsertId()
	if err := replaceCategoryLibraries(r.Context(), tx, id, in.LibraryIDs); err != nil {
		writeError(w, 400, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "ok": true})
}

func (s *server) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var in libraryCategory
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Kind = normalizeCategoryKind(in.Kind)
	if in.Name == "" || !categorySlugRE.MatchString(in.Slug) {
		writeError(w, 400, errors.New("name and a valid slug are required"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback()
	var system bool
	if err := tx.QueryRowContext(r.Context(), `SELECT system FROM library_categories WHERE id=?`, id).Scan(&system); err != nil {
		writeError(w, 404, err)
		return
	}
	if system {
		_, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET name=?,kind=?,sort_order=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.Name, in.Kind, in.SortOrder, in.Active, id)
	} else {
		_, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET name=?,slug=?,kind=?,sort_order=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.Name, in.Slug, in.Kind, in.SortOrder, in.Active, id)
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := replaceCategoryLibraries(r.Context(), tx, id, in.LibraryIDs); err != nil {
		writeError(w, 400, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var system bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT system FROM library_categories WHERE id=?`, id).Scan(&system); err != nil {
		writeError(w, 404, err)
		return
	}
	if system {
		writeError(w, 400, errors.New("system categories cannot be deleted"))
		return
	}
	_, err = s.db.ExecContext(r.Context(), `DELETE FROM library_categories WHERE id=?`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) categories(ctx context.Context, includeInactive bool) ([]libraryCategory, error) {
	q := `SELECT id,name,slug,kind,sort_order,active,system FROM library_categories`
	if !includeInactive {
		q += ` WHERE active=1`
	}
	q += ` ORDER BY sort_order,id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []libraryCategory{}
	for rows.Next() {
		var c libraryCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Kind, &c.SortOrder, &c.Active, &c.System); err != nil {
			return nil, err
		}
		c.LibraryIDs, _ = s.categoryLibraries(ctx, c.ID)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *server) categoryLibraries(ctx context.Context, categoryID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT library_id FROM library_category_libraries WHERE category_id=? ORDER BY sort_order,library_id`, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func replaceCategoryLibraries(ctx context.Context, tx *sql.Tx, categoryID int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_category_libraries WHERE category_id=?`, categoryID); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for i, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_category_libraries(category_id,library_id,sort_order) VALUES(?,?,?)`, categoryID, id, i); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCategoryKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "movie", "series", "anime", "mixed", "other":
		return value
	default:
		return "mixed"
	}
}

func intersectIDs(a, b []int64) []int64 {
	allowed := map[int64]bool{}
	for _, id := range b {
		allowed[id] = true
	}
	out := []int64{}
	for _, id := range a {
		if allowed[id] {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
