package media

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/danilostorm/stormflix/internal/database"
)

func TestHomeQueryV2FiftyThousandColdAndWarmSLO(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO catalog_entities(entity_key,entity_type,representative_media_id,library_id,library_name,title,modified_unix,genres_json,rating,backdrop_url) VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	genreStatement, err := tx.Prepare(`INSERT INTO catalog_entity_genres(entity_key,genre,library_id,modified_unix) VALUES(?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 50000; index++ {
		kind := "media"
		if index%7 == 0 {
			kind = "series"
		}
		if _, err := statement.Exec(fmt.Sprintf("item:%d", index), kind, index, 1, "Biblioteca", fmt.Sprintf("Título %05d", index), index, `["Drama","Ação"]`, float64(index%90)/10, "/assets/backdrops/example.jpg"); err != nil {
			t.Fatal(err)
		}
		for _, genre := range []string{"Drama", "Ação"} {
			if _, err := genreStatement.Exec(fmt.Sprintf("item:%d", index), genre, 1, index); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = statement.Close()
	_ = genreStatement.Close()
	if _, err := tx.Exec(`UPDATE catalog_projection_state SET source_revision=1,built_revision=1,built_at=CURRENT_TIMESTAMP WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	started := time.Now()
	feed, cache, err := service.HomeGroupedCachedWithStatus(context.Background(), nil, "featured", "StormFlix", true, 20, true)
	cold := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if cache.State != "miss" || len(feed.Rows) == 0 {
		t.Fatalf("cold Home state=%+v rows=%d", cache, len(feed.Rows))
	}
	// Measure enough cache-cold application calls for p95 to tolerate one
	// scheduler/filesystem warm-up outlier on shared CI runners. The first call
	// still participates in the sample; only the in-process Home response cache
	// is bypassed below.
	coldSamples := []time.Duration{cold}
	for sample := 1; sample < 20; sample++ {
		started = time.Now()
		if _, err := service.HomeGrouped(context.Background(), nil, "featured", "StormFlix", true, 20, true); err != nil {
			t.Fatal(err)
		}
		coldSamples = append(coldSamples, time.Since(started))
	}
	sort.Slice(coldSamples, func(i, j int) bool { return coldSamples[i] < coldSamples[j] })
	coldP95 := coldSamples[(len(coldSamples)*95-1)/100]
	if coldP95 >= 1500*time.Millisecond {
		t.Fatalf("cold Home p95=%s exceeds 1.5s target; samples=%v", coldP95, coldSamples)
	}
	warmSamples := make([]time.Duration, 0, 20)
	for sample := 0; sample < 20; sample++ {
		started = time.Now()
		_, cache, err = service.HomeGroupedCachedWithStatus(context.Background(), nil, "featured", "StormFlix", true, 20, true)
		warmSamples = append(warmSamples, time.Since(started))
		if err != nil || cache.State != "hit" {
			t.Fatalf("cached Home state=%+v err=%v", cache, err)
		}
	}
	sort.Slice(warmSamples, func(i, j int) bool { return warmSamples[i] < warmSamples[j] })
	warmP95 := warmSamples[(len(warmSamples)*95-1)/100]
	if warmP95 >= 500*time.Millisecond {
		t.Fatalf("cached Home p95=%s exceeds 500ms target; samples=%v", warmP95, warmSamples)
	}
	t.Logf("50k Home SLO: cold p95=%s, cached p95=%s", coldP95, warmP95)
	t.Logf("50k SQL timings: %+v", database.QueryTelemetrySnapshot())
	for _, row := range feed.Rows {
		if len(row.Items) > homeItemsPerRow {
			t.Fatalf("row %s returned %d cards; cold Home is not bounded", row.ID, len(row.Items))
		}
	}
}
