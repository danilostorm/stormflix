package workload

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestGatePausesAndResumesForPlayback(t *testing.T) {
	db, err := sql.Open("sqlite", "file:workload-gate?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE playback_sessions(last_seen_at TEXT); CREATE TABLE game_play_sessions(last_seen_at TEXT); INSERT INTO playback_sessions VALUES(CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	gate := For(db)
	gate.poll = 5 * time.Millisecond
	states := make(chan bool, 2)
	done := make(chan error, 1)
	go func() { done <- gate.Wait(context.Background(), "scan", func(paused bool) { states <- paused }) }()
	select {
	case paused := <-states:
		if !paused {
			t.Fatal("first transition must pause")
		}
	case <-time.After(time.Second):
		t.Fatal("gate did not pause")
	}
	if _, err := db.Exec(`DELETE FROM playback_sessions`); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("gate did not resume")
	}
	if resumed := <-states; resumed {
		t.Fatal("second transition must resume")
	}
	snapshot := gate.Snapshot(context.Background())
	if snapshot.TotalPauses != 1 || len(snapshot.ActiveWaiters) != 0 || snapshot.CategoryPauses["scan"] != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}
