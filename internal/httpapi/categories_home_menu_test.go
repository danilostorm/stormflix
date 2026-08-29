package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/media"
)

func TestOrganizeRecommendedCategoriesMovesDrawingsToOwnHomeMenu(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	animation, err := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES('Desenhos HD','animation_series','/media/desenhos',1)`)
	if err != nil {
		t.Fatalf("insert animation library: %v", err)
	}
	animationID, _ := animation.LastInsertId()

	var seriesRoot int64
	if err := db.QueryRow(`SELECT id FROM library_categories WHERE slug='series'`).Scan(&seriesRoot); err != nil {
		t.Fatalf("series root: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO library_categories(name,slug,kind,parent_id,sort_order,active,system) VALUES('Animes com temporadas','series-animes','anime',?,30,1,0)`, seriesRoot); err != nil {
		t.Fatalf("insert legacy anime section: %v", err)
	}

	s := &server{db: db, admin: admin.NewService(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories/organize", nil)
	rec := httptest.NewRecorder()
	s.organizeRecommendedCategories(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var drawingsRoot int64
	var parent sql.NullInt64
	if err := db.QueryRow(`SELECT id,parent_id FROM library_categories WHERE slug='desenhos'`).Scan(&drawingsRoot, &parent); err != nil {
		t.Fatalf("drawings Home menu: %v", err)
	}
	if parent.Valid {
		t.Fatalf("drawings menu unexpectedly has parent %d", parent.Int64)
	}

	var drawingsSection, sectionParent int64
	if err := db.QueryRow(`SELECT id,parent_id FROM library_categories WHERE slug='series-desenhos' AND active=1`).Scan(&drawingsSection, &sectionParent); err != nil {
		t.Fatalf("drawings gallery section: %v", err)
	}
	if sectionParent != drawingsRoot {
		t.Fatalf("drawings section parent=%d want %d", sectionParent, drawingsRoot)
	}

	var linked int
	if err := db.QueryRow(`SELECT COUNT(*) FROM library_category_libraries WHERE category_id=? AND library_id=?`, drawingsSection, animationID).Scan(&linked); err != nil {
		t.Fatalf("drawings section assignment: %v", err)
	}
	if linked != 1 {
		t.Fatalf("animation library link=%d want 1", linked)
	}

	var legacyActive int
	if err := db.QueryRow(`SELECT active FROM library_categories WHERE slug='series-animes'`).Scan(&legacyActive); err != nil {
		t.Fatalf("legacy series anime section: %v", err)
	}
	if legacyActive != 0 {
		t.Fatalf("legacy series anime section active=%d want 0", legacyActive)
	}
}

func TestGallerySectionUsesSelectedLibraryWithoutKindHidingAnimeMovies(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	authService := auth.NewService(db)
	user, err := authService.CreateFirstAdmin(context.Background(), "admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	library, err := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES('Filmes Animes','mixed','/media/filmes-animes',1)`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := library.LastInsertId()

	movie, err := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(?,?,?,?,0,0,1)`, libraryID, "Filme Anime", "/media/filmes-animes/filme-anime.mkv", ".mkv")
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	mediaID, _ := movie.LastInsertId()
	if _, err := db.Exec(`INSERT INTO media_metadata(media_id,media_type,year,season_number,episode_number,provider,provider_id,tmdb_id,status,manual_match) VALUES(?, 'movie', 2025, 0, 0, 'tmdb', '77', 77, 'matched', 0)`, mediaID); err != nil {
		t.Fatalf("insert movie metadata: %v", err)
	}

	var animeRoot int64
	if err := db.QueryRow(`SELECT id FROM library_categories WHERE slug='anime'`).Scan(&animeRoot); err != nil {
		t.Fatalf("anime root: %v", err)
	}
	section, err := db.Exec(`INSERT INTO library_categories(name,slug,kind,parent_id,sort_order,active,system) VALUES('Filmes Animes','filmes-animes','anime',?,20,1,0)`, animeRoot)
	if err != nil {
		t.Fatalf("insert gallery section: %v", err)
	}
	sectionID, _ := section.LastInsertId()
	if _, err := db.Exec(`INSERT INTO library_category_libraries(category_id,library_id,sort_order) VALUES(?,?,0)`, sectionID, libraryID); err != nil {
		t.Fatalf("assign section library: %v", err)
	}

	s := &server{db: db, media: media.NewService(db), auth: authService}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/filmes-animes", nil)
	req.SetPathValue("slug", "filmes-animes")
	req = req.WithContext(context.WithValue(req.Context(), userKey, user))
	rec := httptest.NewRecorder()
	s.browseCategory(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Media []media.Item `json:"media"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode category response: %v", err)
	}
	if len(response.Media) != 1 {
		t.Fatalf("media=%d want 1 body=%s", len(response.Media), rec.Body.String())
	}
	if response.Media[0].ID != mediaID {
		t.Fatalf("media id=%d want %d", response.Media[0].ID, mediaID)
	}
}
