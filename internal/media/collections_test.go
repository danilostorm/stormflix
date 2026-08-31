package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestCollectionsGroupsMoviesAndRespectsLibraryAccess(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	res, err := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES('Filmes','movies','/films',1)`)
	if err != nil {
		t.Fatal(err)
	}
	allowedLibrary, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES('Privado','movies','/private',1)`)
	if err != nil {
		t.Fatal(err)
	}
	privateLibrary, _ := res.LastInsertId()

	insertMovie := func(libraryID int64, title string, year int, tmdbID, collectionID int64, collectionName string) {
		t.Helper()
		m, err := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(?,?,?,?,0,1,1)`, libraryID, title, "/"+title+".mkv", ".mkv")
		if err != nil {
			t.Fatal(err)
		}
		mediaID, _ := m.LastInsertId()
		_, err = db.Exec(`INSERT INTO media_metadata(media_id,media_type,year,tmdb_id,status,collection_tmdb_id,collection_name,collection_source_tmdb_id,collection_checked_at) VALUES(?,'movie',?,?, 'matched',?,?,?,CURRENT_TIMESTAMP)`, mediaID, year, tmdbID, collectionID, collectionName, tmdbID)
		if err != nil {
			t.Fatal(err)
		}
	}

	insertMovie(allowedLibrary, "Resident Evil", 2002, 1, 100, "Resident Evil Collection")
	insertMovie(allowedLibrary, "Resident Evil: Apocalypse", 2004, 2, 100, "Resident Evil Collection")
	insertMovie(allowedLibrary, "Filme Solo", 2020, 3, 200, "Single Collection")
	insertMovie(privateLibrary, "Resident Evil: Private", 2007, 4, 100, "Resident Evil Collection")

	service := NewService(db)
	collections, err := service.Collections(context.Background(), []int64{allowedLibrary}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 {
		t.Fatalf("expected one visible collection, got %+v", collections)
	}
	collection := collections[0]
	if collection.TMDBID != 100 || collection.ItemCount != 2 {
		t.Fatalf("unexpected collection: %+v", collection)
	}
	if collection.Items[0].Title != "Resident Evil" || collection.Items[1].Title != "Resident Evil: Apocalypse" {
		t.Fatalf("expected chronological order, got %+v", collection.Items)
	}
	for _, item := range collection.Items {
		if item.LibraryID == privateLibrary {
			t.Fatalf("private library leaked into collection: %+v", item)
		}
	}
}
