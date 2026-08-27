package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type categoryLibraryCandidate struct {
	ID   int64
	Name string
	Kind string
}

// organizeRecommendedCategories creates a useful first hierarchy from the
// library kinds/names already configured. It only manages the recommended child
// slugs below the three system roots; custom categories are left untouched.
func (s *server) organizeRecommendedCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,name,kind FROM libraries WHERE enabled=1 ORDER BY name,id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	libs := []categoryLibraryCandidate{}
	for rows.Next() {
		var item categoryLibraryCandidate
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		libs = append(libs, item)
	}
	_ = rows.Close()

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	rootID := func(slug string) (int64, error) {
		var id int64
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM library_categories WHERE slug=? AND system=1`, slug).Scan(&id)
		return id, err
	}
	movieRoot, err := rootID("movie")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	seriesRoot, err := rootID("series")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	animeRoot, err := rootID("anime")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	type spec struct {
		parent int64
		name, slug, kind string
		sort int
		match func(categoryLibraryCandidate) bool
	}
	lowerName := func(v categoryLibraryCandidate) string { return strings.ToLower(strings.TrimSpace(v.Name)) }
	containsAny := func(value string, terms ...string) bool {
		for _, term := range terms {
			if strings.Contains(value, term) {
				return true
			}
		}
		return false
	}
	specs := []spec{
		{movieRoot, "4K / UHD", "filmes-4k", "movie", 10, func(l categoryLibraryCandidate) bool { n:=lowerName(l); return (l.Kind=="movies"||l.Kind=="mixed") && containsAny(n,"4k","uhd","2160") }},
		{movieRoot, "Animação", "filmes-animacao", "movie", 20, func(l categoryLibraryCandidate) bool { n:=lowerName(l); return (l.Kind=="movies"||l.Kind=="mixed") && containsAny(n,"anima","desenho","cartoon") }},
		{movieRoot, "Outros filmes", "filmes-outros", "movie", 90, func(l categoryLibraryCandidate) bool { n:=lowerName(l); return (l.Kind=="movies"||l.Kind=="mixed") && !containsAny(n,"4k","uhd","2160","anima","desenho","cartoon","anime") }},
		{seriesRoot, "Séries de TV", "series-tv", "series", 10, func(l categoryLibraryCandidate) bool { return l.Kind=="series" }},
		{seriesRoot, "Desenhos", "series-desenhos", "series", 20, func(l categoryLibraryCandidate) bool { return l.Kind=="animation_series" }},
		{seriesRoot, "Animes com temporadas", "series-animes", "anime", 30, func(l categoryLibraryCandidate) bool { return l.Kind=="anime_series" }},
		{animeRoot, "Dublados", "animes-dublados", "anime", 10, func(l categoryLibraryCandidate) bool { n:=lowerName(l); return (l.Kind=="anime"||l.Kind=="anime_series"||l.Kind=="mixed") && containsAny(n,"dubl") }},
		{animeRoot, "Séries", "animes-series", "anime", 20, func(l categoryLibraryCandidate) bool { return l.Kind=="anime_series" }},
		{animeRoot, "Filmes", "animes-filmes", "anime", 30, func(l categoryLibraryCandidate) bool { n:=lowerName(l); return (l.Kind=="anime"||l.Kind=="mixed") && !containsAny(n,"serie","temporada") }},
	}
	created := 0
	assigned := 0
	for _, sp := range specs {
		var categoryID int64
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM library_categories WHERE slug=?`, sp.slug).Scan(&categoryID)
		if errors.Is(err, sql.ErrNoRows) {
			res, insertErr := tx.ExecContext(r.Context(), `INSERT INTO library_categories(name,slug,kind,parent_id,sort_order,active,system) VALUES(?,?,?,?,?,1,0)`, sp.name, sp.slug, sp.kind, sp.parent, sp.sort)
			if insertErr != nil {
				writeError(w, http.StatusBadRequest, insertErr)
				return
			}
			categoryID, _ = res.LastInsertId()
			created++
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		} else {
			_, _ = tx.ExecContext(r.Context(), `UPDATE library_categories SET name=?,kind=?,parent_id=?,sort_order=?,active=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, sp.name, sp.kind, sp.parent, sp.sort, categoryID)
		}
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM library_category_libraries WHERE category_id=?`, categoryID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		order := 0
		for _, lib := range libs {
			if !sp.match(lib) {
				continue
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO library_category_libraries(category_id,library_id,sort_order) VALUES(?,?,?)`, categoryID, lib.ID, order); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			order++
			assigned++
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "categories", "Estrutura recomendada de categorias/subcategorias organizada", &uid, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "created": created, "assignments": assigned})
}
