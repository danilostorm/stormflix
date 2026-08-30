package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/database"
)

func TestProfileHomeMenusPersistAndFollowSelectedProfile(t *testing.T) {
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
	profiles, err := authService.Profiles(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) == 0 {
		profile, createErr := authService.CreateProfile(context.Background(), user.ID, "Principal", "storm-red", "", false)
		if createErr != nil {
			t.Fatalf("create profile: %v", createErr)
		}
		profiles = append(profiles, profile)
	}
	profileID := profiles[0].ID

	var movieID, animeID int64
	if err := db.QueryRow(`SELECT id FROM library_categories WHERE slug='movie' AND parent_id IS NULL`).Scan(&movieID); err != nil {
		t.Fatalf("movie root: %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM library_categories WHERE slug='anime' AND parent_id IS NULL`).Scan(&animeID); err != nil {
		t.Fatalf("anime root: %v", err)
	}

	s := &server{db: db, auth: authService}
	body, _ := json.Marshal(map[string]any{"menus": []map[string]any{
		{"category_id": movieID, "visible": false, "sort_order": 20},
		{"category_id": animeID, "visible": true, "sort_order": 10},
	}})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/profiles/1/home-menus", bytes.NewReader(body))
	updateReq.SetPathValue("id", strconv.FormatInt(profileID, 10))
	updateReq = updateReq.WithContext(context.WithValue(updateReq.Context(), userKey, user))
	updateRec := httptest.NewRecorder()
	s.updateProfileHomeMenus(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/home-menus", nil)
	getReq.AddCookie(&http.Cookie{Name: profileCookie, Value: strconv.FormatInt(profileID, 10)})
	getReq = getReq.WithContext(context.WithValue(getReq.Context(), userKey, user))
	getRec := httptest.NewRecorder()
	s.selectedProfileHomeMenus(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var entries []profileHomeMenuEntry
	if err := json.Unmarshal(getRec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2: %#v", len(entries), entries)
	}
	if entries[0].CategoryID != animeID || !entries[0].Visible || entries[0].SortOrder != 10 {
		t.Fatalf("first entry=%#v want visible anime at order 10", entries[0])
	}
	if entries[1].CategoryID != movieID || entries[1].Visible || entries[1].SortOrder != 20 {
		t.Fatalf("second entry=%#v want hidden movie at order 20", entries[1])
	}
}
