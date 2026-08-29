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

// organizeRecommendedCategories builds the two-level presentation model used by
// the Web Home: root categories are Home menu buttons and their direct children
// are gallery sections. Custom items are never deleted. Reserved recommended
// slugs may be moved/renamed so installations created by the older hierarchy
// converge to the current Home-menu model.
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
		err := tx.QueryRowContext(r.Context(), `SELECT id FROM library_categories WHERE slug=? AND parent_id IS NULL`, slug).Scan(&id)
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

	created := 0
	assigned := 0
	hasAnimation := false
	for _, lib := range libs {
		if lib.Kind == "animation_series" {
			hasAnimation = true
			break
		}
	}

	var drawingsRoot int64
	if hasAnimation {
		drawingsRoot, err = rootID("desenhos")
		if errors.Is(err, sql.ErrNoRows) {
			res, insertErr := tx.ExecContext(r.Context(), `INSERT INTO library_categories(name,slug,kind,parent_id,sort_order,active,system) VALUES('Desenhos','desenhos','series',NULL,40,1,0)`)
			if insertErr != nil {
				writeError(w, http.StatusBadRequest, insertErr)
				return
			}
			drawingsRoot, _ = res.LastInsertId()
			created++
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		} else {
			_, _ = tx.ExecContext(r.Context(), `UPDATE library_categories SET kind='series',sort_order=40,active=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, drawingsRoot)
		}
	}

	type spec struct {
		parent           int64
		name, slug, kind string
		sort             int
		match            func(categoryLibraryCandidate) bool
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
	isMovieLib := func(l categoryLibraryCandidate) bool { return l.Kind == "movies" || l.Kind == "mixed" }
	isAnimeLib := func(l categoryLibraryCandidate) bool { return l.Kind == "anime" || l.Kind == "anime_series" || l.Kind == "mixed" }
	is4K := func(l categoryLibraryCandidate) bool { return containsAny(lowerName(l), "4k", "uhd", "2160") }
	isAnimationMovie := func(l categoryLibraryCandidate) bool { return containsAny(lowerName(l), "anima", "desenho", "cartoon") }
	isDubbedAnime := func(l categoryLibraryCandidate) bool { return containsAny(lowerName(l), "dubl") }

	specs := []spec{
		{movieRoot, "4K / UHD", "filmes-4k", "movie", 10, func(l categoryLibraryCandidate) bool { return isMovieLib(l) && is4K(l) }},
		{movieRoot, "Animação", "filmes-animacao", "movie", 20, func(l categoryLibraryCandidate) bool { return isMovieLib(l) && !is4K(l) && isAnimationMovie(l) }},
		{movieRoot, "Outros filmes", "filmes-outros", "movie", 90, func(l categoryLibraryCandidate) bool { return isMovieLib(l) && !is4K(l) && !isAnimationMovie(l) && !strings.Contains(lowerName(l), "anime") }},
		{seriesRoot, "Séries de TV", "series-tv", "series", 10, func(l categoryLibraryCandidate) bool { return l.Kind == "series" }},
		{animeRoot, "Dublados", "animes-dublados", "anime", 10, func(l categoryLibraryCandidate) bool { return isAnimeLib(l) && isDubbedAnime(l) }},
		{animeRoot, "Séries", "animes-series", "anime", 20, func(l categoryLibraryCandidate) bool { return l.Kind == "anime_series" && !isDubbedAnime(l) }},
		{animeRoot, "Filmes", "animes-filmes", "anime", 30, func(l categoryLibraryCandidate) bool { return (l.Kind == "anime" || l.Kind == "mixed") && !isDubbedAnime(l) }},
	}
	if drawingsRoot > 0 {
		specs = append(specs, spec{drawingsRoot, "Todos os desenhos", "series-desenhos", "series", 10, func(l categoryLibraryCandidate) bool { return l.Kind == "animation_series" }})
	}

	// This was an older recommended section under Séries. Anime-season content
	// now belongs to the Animes menu, so keep the record for compatibility but
	// hide it when the organizer is explicitly run.
	_, _ = tx.ExecContext(r.Context(), `UPDATE library_categories SET active=0,updated_at=CURRENT_TIMESTAMP WHERE slug='series-animes' AND system=0`)

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
	s.admin.Log(r.Context(), "info", "categories", "Menus da Home e seções recomendadas organizados", &uid, "")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "created": created, "assignments": assigned})
}
