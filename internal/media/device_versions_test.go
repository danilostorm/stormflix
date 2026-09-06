package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestVersionCandidatesGroupsPhysical4KAnd1080pAcrossLibraries(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	insertVersion := func(libraryName, path string, height int, codec string) (int64, int64) {
		t.Helper()
		library, insertErr := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES(?,'movies',?,1)`, libraryName, "/"+libraryName)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		libraryID, _ := library.LastInsertId()
		row, insertErr := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(?,'Filme Teste',?,'.mkv',1000,1,1)`, libraryID, path)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		mediaID, _ := row.LastInsertId()
		if _, insertErr = db.Exec(`INSERT INTO media_metadata(media_id,media_type,year,tmdb_id,status) VALUES(?,'movie',2024,9001,'matched')`, mediaID); insertErr != nil {
			t.Fatal(insertErr)
		}
		if _, insertErr = db.Exec(`INSERT INTO media_technical(media_id,status,height,width,video_codec,source_modified_unix) VALUES(?,'ok',?,?,?,1)`, mediaID, height, height*16/9, codec); insertErr != nil {
			t.Fatal(insertErr)
		}
		return mediaID, libraryID
	}

	uhdID, uhdLibrary := insertVersion("Filmes 4K", "/4k/filme.mkv", 2160, "hevc")
	fhdID, fhdLibrary := insertVersion("Filmes", "/1080/filme.mkv", 1080, "h264")
	candidates, err := NewService(db).VersionCandidates(context.Background(), []int64{uhdID}, []int64{uhdLibrary, fhdLibrary})
	if err != nil {
		t.Fatal(err)
	}
	got := candidates[uhdID]
	if len(got) != 2 || got[0].ID != uhdID || got[1].ID != fhdID || got[1].Height != 1080 {
		t.Fatalf("unexpected grouped versions: %#v", got)
	}
	service := NewService(db)
	for _, tc := range []struct {
		name    string
		allowed []int64
		count   int
	}{
		{"alternate library denied", []int64{uhdLibrary}, 1},
		{"requested library denied", []int64{fhdLibrary}, 0},
		{"empty permissions", []int64{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.VersionCandidates(context.Background(), []int64{uhdID}, tc.allowed)
			if err != nil || len(result[uhdID]) != tc.count {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
	if _, err = db.Exec(`UPDATE media_metadata SET media_type='tv',season_number=1,episode_number=CASE WHEN media_id=? THEN 1 ELSE 2 END`, uhdID); err != nil {
		t.Fatal(err)
	}
	candidates, err = service.VersionCandidates(context.Background(), []int64{uhdID, fhdID}, nil)
	if err != nil || len(candidates[uhdID]) != 1 || len(candidates[fhdID]) != 1 {
		t.Fatalf("different episodes must not be substituted: %+v %v", candidates, err)
	}
	if _, err = db.Exec(`UPDATE media_metadata SET tmdb_id=0,media_type='movie',season_number=0,episode_number=0`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE media SET title='ÁGUA E FOGO'`); err != nil {
		t.Fatal(err)
	}
	candidates, err = service.VersionCandidates(context.Background(), []int64{uhdID}, nil)
	if err != nil || len(candidates[uhdID]) != 2 {
		t.Fatalf("unmatched accented title disappeared: %+v %v", candidates, err)
	}
	if _, err = db.Exec(`UPDATE media SET modified_unix=2 WHERE id=?`, uhdID); err != nil {
		t.Fatal(err)
	}
	candidates, err = service.VersionCandidates(context.Background(), []int64{uhdID}, nil)
	if err != nil || candidates[uhdID][0].TechnicalStatus != "pending" {
		t.Fatalf("stale technical row trusted: %+v %v", candidates, err)
	}
}
