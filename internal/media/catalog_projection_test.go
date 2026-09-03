package media

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestCatalogProjectionBuildsLogicalHomeAndRespectsLibraryScope(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	movies := insertProjectionLibrary(t, db, "Filmes", "movies", "/movies")
	series := insertProjectionLibrary(t, db, "Séries", "series", "/series")
	private := insertProjectionLibrary(t, db, "Privado", "movies", "/private")
	movieA := insertProjectionMedia(t, db, movies, "Top Gun", "/movies/top-gun-a.mkv", "movie", 1986, 100, 0, 0, 100)
	insertProjectionMedia(t, db, movies, "Top Gun 1080p", "/movies/top-gun-b.mkv", "movie", 1986, 100, 0, 0, 99)
	episode1 := insertProjectionMedia(t, db, series, "Pilot S01E01", "/series/Example/Season 1/e01.mkv", "series", 2025, 500, 1, 1, 200)
	episode2 := insertProjectionMedia(t, db, series, "Next S01E02", "/series/Example/Season 1/e02.mkv", "series", 2025, 500, 1, 2, 201)
	insertProjectionIdentity(t, db, episode1, series, "example", 1, 1)
	insertProjectionIdentity(t, db, episode2, series, "example", 1, 2)
	insertProjectionMedia(t, db, private, "Secret", "/private/secret.mkv", "movie", 2025, 900, 0, 0, 300)

	service := NewService(db)
	feed, err := service.HomeGrouped(context.Background(), []int64{movies, series}, "featured", "StormFlix", true, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	items := uniqueItemsFromFeed(feed)
	if len(items) != 4 { // movie + series card + the two explicit recent episodes
		t.Fatalf("logical Home items=%d, want 4: %+v", len(items), items)
	}
	var movieCount, seriesCount, secretCount int
	for _, item := range items {
		switch {
		case item.TMDBID == 100 && item.EntityType != "series":
			movieCount++
		case item.EntityType == "series":
			seriesCount++
		case item.LibraryID == private:
			secretCount++
		}
	}
	if movieCount != 1 || seriesCount != 1 || secretCount != 0 {
		t.Fatalf("unexpected logical projection movie=%d series=%d private=%d items=%+v", movieCount, seriesCount, secretCount, items)
	}
	if feed.Hero == nil || isEpisodeItem(*feed.Hero) {
		t.Fatalf("raw episode selected as Home hero: %+v", feed.Hero)
	}
	logical, err := service.CatalogList(context.Background(), 0, "", 500, 0, []int64{movies, series})
	if err != nil {
		t.Fatal(err)
	}
	if len(logical) != 2 {
		t.Fatalf("logical catalog cards=%d, want movie + series: %+v", len(logical), logical)
	}
	shows, err := service.SeriesList(context.Background(), []int64{series}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(shows) != 1 || shows[0].EpisodeCount != 2 || shows[0].SeasonCount != 1 {
		t.Fatalf("projected series list=%+v", shows)
	}
	filteredShows, err := service.SeriesList(context.Background(), []int64{series}, shows[0].Title)
	if err != nil || len(filteredShows) != 1 {
		t.Fatalf("projected series search title=%q items=%+v err=%v", shows[0].Title, filteredShows, err)
	}

	resolved, err := service.projectedItemsFromMediaIDs(context.Background(), []int64{episode2, movieA}, []int64{movies, series}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[0].EntityType != "series" || resolved[1].TMDBID != 100 {
		t.Fatalf("discovery IDs were not resolved to logical entities: %+v", resolved)
	}

	status, err := service.CatalogProjectionStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.BuiltRevision != status.SourceRevision || status.EntityCount != 3 {
		t.Fatalf("projection status after build: %+v", status)
	}
	firstCached, firstCache, err := service.HomeGroupedCachedWithStatus(context.Background(), []int64{movies, series}, "featured", "StormFlix", true, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	secondCached, secondCache, err := service.HomeGroupedCachedWithStatus(context.Background(), []int64{series, movies}, "featured", "StormFlix", true, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	if firstCache.State != "miss" || secondCache.State != "hit" || firstCached.CacheRevision == 0 || secondCached.CacheRevision != firstCached.CacheRevision {
		t.Fatalf("revision-aware Home cache first=%+v second=%+v revisions=%d/%d", firstCache, secondCache, firstCached.CacheRevision, secondCached.CacheRevision)
	}
	if _, err := db.Exec(`UPDATE media SET title='Top Gun atualizado' WHERE id=?`, movieA); err != nil {
		t.Fatal(err)
	}
	dirty, err := service.CatalogProjectionStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dirty.SourceRevision <= dirty.BuiltRevision {
		t.Fatalf("catalog mutation did not invalidate projection: %+v", dirty)
	}
}

func TestNextEpisodeQueryDoesNotRequireFullSeriesRegroup(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	series := insertProjectionLibrary(t, db, "Séries", "series", "/series")
	episode1 := insertProjectionMedia(t, db, series, "Episode 1", "/series/Show/e01.mkv", "series", 2025, 700, 1, 1, 100)
	episode2 := insertProjectionMedia(t, db, series, "Episode 2", "/series/Show/e02.mkv", "series", 2025, 700, 1, 2, 101)
	insertProjectionIdentity(t, db, episode1, series, "show", 1, 1)
	insertProjectionIdentity(t, db, episode2, series, "show", 1, 2)
	if _, err := db.Exec(`INSERT INTO users(username,display_name,password_hash,role) VALUES('viewer','Viewer','hash','user')`); err != nil {
		t.Fatal(err)
	}
	var profileID int64
	if err := db.QueryRow(`SELECT id FROM profiles ORDER BY id DESC LIMIT 1`).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profile_progress(profile_id,media_id,position_seconds,duration_seconds,completed,updated_at) VALUES(?,?,1800,1800,1,CURRENT_TIMESTAMP)`, profileID, episode1); err != nil {
		t.Fatal(err)
	}

	items, err := NewService(db).ContinueWatching(context.Background(), profileID, []int64{series}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != episode2 {
		t.Fatalf("Continue Watching=%+v, want next episode %d", items, episode2)
	}
}

func insertProjectionLibrary(t *testing.T, db *sql.DB, name, kind, path string) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO libraries(name,kind,path,enabled) VALUES(?,?,?,1)`, name, kind, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertProjectionMedia(t *testing.T, db *sql.DB, libraryID int64, title, path, mediaType string, year int, tmdbID int64, season, episode int, modified int64) int64 {
	t.Helper()
	result, err := db.Exec(`INSERT INTO media(library_id,title,path,extension,size_bytes,modified_unix,available) VALUES(?,?,?,'.mkv',1000,?,1)`, libraryID, title, path, modified)
	if err != nil {
		t.Fatal(err)
	}
	mediaID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media_metadata(media_id,media_type,year,season_number,episode_number,genres_json,rating,status,tmdb_id,release_date) VALUES(?,?,?,?,?,'["Drama"]',8.2,'matched',?,'2025-01-01')`, mediaID, mediaType, year, season, episode, tmdbID); err != nil {
		t.Fatal(err)
	}
	return mediaID
}

func insertProjectionIdentity(t *testing.T, db *sql.DB, mediaID, libraryID int64, seriesKey string, season, episode int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO media_series_identity(media_id,library_id,source_root,series_key,series_title,season_number,episode_number,absolute_number) VALUES(?,?,?,?,?,?,?,?)`,
		mediaID, libraryID, "/series", seriesKey, "Example Show", season, episode, episode); err != nil {
		t.Fatal(err)
	}
}

func uniqueItemsFromFeed(feed HomeFeed) []Item {
	out := []Item{}
	seen := map[string]bool{}
	for _, row := range feed.Rows {
		for _, item := range row.Items {
			key := fmt.Sprintf("%d:%s:%s:%d:%d", item.LibraryID, item.EntityType, item.SeriesID, item.ID, item.EpisodeNumber)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}
