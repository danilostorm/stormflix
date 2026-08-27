package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestAdminCatalogWorksGroupsEpisodesIntoOnePrincipalSeries(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	res, err := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES('Desenhos','animation_series','/media/Desenhos',1)`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := res.LastInsertId()

	insertEpisode := func(path string, episode int) int64 {
		result, err := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(?,?,?,?,0,0,1)`, libraryID, "arquivo", path, ".mkv")
		if err != nil {
			t.Fatalf("insert media: %v", err)
		}
		mediaID, _ := result.LastInsertId()
		if _, err := db.Exec(`INSERT INTO media_series_identity(media_id,library_id,source_root,series_key,series_title,season_number,episode_number,absolute_number) VALUES(?,?,?,?,?,?,?,?)`, mediaID, libraryID, "/media/Desenhos", "pica-pau-e-seus-amigos", "Pica-Pau e seus Amigos", 1, episode, episode); err != nil {
			t.Fatalf("insert series identity: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO media_metadata(media_id,media_type,year,season_number,episode_number,provider,provider_id,tmdb_id,status,manual_match) VALUES(?,?,?,?,?,'tmdb','123',123,'matched',0)`, mediaID, "series", 1957, 1, episode); err != nil {
			t.Fatalf("insert metadata: %v", err)
		}
		return mediaID
	}

	insertEpisode("/media/Desenhos/Pica-Pau e seus Amigos/Remux/001PP.mkv", 1)
	insertEpisode("/media/Desenhos/Pica-Pau e seus Amigos/Remux/002PP.mkv", 2)

	s := &server{db: db}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/catalog/works?library_id="+strconv.FormatInt(libraryID, 10), nil)
	rec := httptest.NewRecorder()
	s.adminCatalogWorks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var works []adminCatalogWork
	if err := json.Unmarshal(rec.Body.Bytes(), &works); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("works=%d want 1: %s", len(works), rec.Body.String())
	}
	if works[0].EntityType != "series" {
		t.Fatalf("entity_type=%q want series", works[0].EntityType)
	}
	if works[0].Title != "Pica-Pau e seus Amigos" {
		t.Fatalf("title=%q", works[0].Title)
	}
	if works[0].SeasonCount != 1 || works[0].EpisodeCount != 2 {
		t.Fatalf("season/episode count=%d/%d want 1/2", works[0].SeasonCount, works[0].EpisodeCount)
	}
}
