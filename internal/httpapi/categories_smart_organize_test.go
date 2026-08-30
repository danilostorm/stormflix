package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/database"
)

func TestOrganizeRecommendedCategoriesPersistsTechnicalSmartShelves(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	s := &server{db: db, admin: admin.NewService(db)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories/organize", nil)
	rec := httptest.NewRecorder()
	s.organizeRecommendedCategories(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	checks := []struct {
		slug            string
		wantDub         string
		wantAudioPTBR   bool
		wantSubtitlePT  bool
		wantMinHeight   int
	}{
		{slug: "filmes-4k", wantMinHeight: 2000},
		{slug: "animes-dublados", wantDub: "dublado", wantAudioPTBR: true},
		{slug: "animes-legendados", wantDub: "legendado", wantSubtitlePT: true},
	}
	for _, check := range checks {
		var mode, raw string
		if err := db.QueryRow(`SELECT rule_mode,rules_json FROM library_categories WHERE slug=? AND active=1`, check.slug).Scan(&mode, &raw); err != nil {
			t.Fatalf("read %s smart rule: %v", check.slug, err)
		}
		if mode != "rules" {
			t.Fatalf("%s rule_mode=%q want rules", check.slug, mode)
		}
		var rules categoryRules
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			t.Fatalf("decode %s rules: %v", check.slug, err)
		}
		if rules.DubStatus != check.wantDub || rules.AudioPTBR != check.wantAudioPTBR || rules.SubtitlePTBR != check.wantSubtitlePT || rules.MinHeight != check.wantMinHeight {
			t.Fatalf("%s rules=%#v", check.slug, rules)
		}
	}
}
