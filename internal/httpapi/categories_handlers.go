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
	ParentID   *int64  `json:"parent_id,omitempty"`
	SortOrder  int     `json:"sort_order"`
	Active     bool    `json:"active"`
	System     bool    `json:"system"`
	LibraryIDs []int64 `json:"library_ids"`
	ChildCount int     `json:"child_count"`
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
	visible := make([]libraryCategory, 0, len(items))
	for _, c := range items {
		// Parent nodes may intentionally contain no directly assigned library but
		// still need to be visible when a child contains accessible media.
		if !c.Active {
			continue
		}
		if len(c.LibraryIDs) > 0 || c.ChildCount > 0 {
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
	var parent sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `SELECT id,name,slug,kind,parent_id,sort_order,active,system FROM library_categories WHERE slug=? AND active=1`, slug).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Kind, &parent, &c.SortOrder, &c.Active, &c.System)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("category not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if parent.Valid {
		v := parent.Int64
		c.ParentID = &v
	}

	// Browsing a parent aggregates all descendant libraries. Browsing a child
	// stays scoped to that branch. Direct children are manually configured
	// gallery sections, so their selected libraries define membership; the
	// category kind is a presentation hint and must not hide a movie stored in
	// an anime-film library (or another intentionally mixed library).
	ids, err := s.categoryTreeLibraries(r.Context(), c.ID)
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

	manualSection := c.ParentID != nil
	if manualSection || c.Kind == "series" || c.Kind == "anime" || c.Kind == "mixed" {
		response.Series, err = s.media.SeriesList(r.Context(), ids, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if manualSection || c.Kind != "series" {
		items, listErr := s.media.CatalogList(r.Context(), 0, "", 500, 0, ids)
		if listErr != nil {
			writeError(w, http.StatusInternalServerError, listErr)
			return
		}
		for _, item := range items {
			if item.EntityType == "series" || item.EpisodeNumber > 0 || item.MediaType == "series" {
				continue
			}
			if !manualSection {
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
	if err := s.validateCategoryParent(r.Context(), 0, in.ParentID); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO library_categories(name,slug,kind,parent_id,sort_order,active,system) VALUES(?,?,?,?,?,?,0)`, in.Name, in.Slug, in.Kind, nullableCategoryID(in.ParentID), in.SortOrder, in.Active)
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
	if err := s.validateCategoryParent(r.Context(), id, in.ParentID); err != nil {
		writeError(w, http.StatusBadRequest, err)
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
		// System roots remain roots; users organize their own children below them.
		_, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET name=?,kind=?,parent_id=NULL,sort_order=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.Name, in.Kind, in.SortOrder, in.Active, id)
	} else {
		_, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET name=?,slug=?,kind=?,parent_id=?,sort_order=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.Name, in.Slug, in.Kind, nullableCategoryID(in.ParentID), in.SortOrder, in.Active, id)
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
	q := `SELECT c.id,c.name,c.slug,c.kind,c.parent_id,c.sort_order,c.active,c.system,(SELECT COUNT(*) FROM library_categories ch WHERE ch.parent_id=c.id AND (? OR ch.active=1)) FROM library_categories c`
	if !includeInactive {
		q += ` WHERE c.active=1`
	}
	q += ` ORDER BY COALESCE(c.parent_id,0),c.sort_order,c.id`
	rows, err := s.db.QueryContext(ctx, q, includeInactive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []libraryCategory{}
	for rows.Next() {
		var c libraryCategory
		var parent sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Kind, &parent, &c.SortOrder, &c.Active, &c.System, &c.ChildCount); err != nil {
			return nil, err
		}
		if parent.Valid {
			v := parent.Int64
			c.ParentID = &v
		}
		// Return aggregated libraries for parent nodes so permissions and clients
		// can tell whether a branch is actually useful without another request.
		c.LibraryIDs, _ = s.categoryTreeLibraries(ctx, c.ID)
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

func (s *server) categoryTreeLibraries(ctx context.Context, categoryID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `WITH RECURSIVE branch(id) AS (
 SELECT id FROM library_categories WHERE id=?
 UNION ALL
 SELECT c.id FROM library_categories c JOIN branch b ON c.parent_id=b.id WHERE c.active=1
)
SELECT DISTINCT lcl.library_id FROM library_category_libraries lcl JOIN branch b ON b.id=lcl.category_id ORDER BY lcl.library_id`, categoryID)
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

func (s *server) validateCategoryParent(ctx context.Context, categoryID int64, parentID *int64) error {
	if parentID == nil || *parentID == 0 {
		return nil
	}
	if categoryID > 0 && *parentID == categoryID {
		return errors.New("a category cannot be its own parent")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_categories WHERE id=?`, *parentID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("parent category not found")
	}
	if categoryID <= 0 {
		return nil
	}
	var cycle int
	if err := s.db.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (
 SELECT id FROM library_categories WHERE parent_id=?
 UNION ALL
 SELECT c.id FROM library_categories c JOIN descendants d ON c.parent_id=d.id
) SELECT COUNT(*) FROM descendants WHERE id=?`, categoryID, *parentID).Scan(&cycle); err != nil {
		return err
	}
	if cycle > 0 {
		return errors.New("category hierarchy cannot contain a cycle")
	}
	return nil
}

func nullableCategoryID(id *int64) any {
	if id == nil || *id <= 0 {
		return nil
	}
	return *id
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
