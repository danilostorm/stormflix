package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/database"
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
