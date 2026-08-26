package admin

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOrderedProgressRejectsLateRequestAndAcceptsBackwardSeek(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE profile_progress (
 profile_id INTEGER NOT NULL,
 media_id INTEGER NOT NULL,
 position_seconds REAL NOT NULL DEFAULT 0,
 duration_seconds REAL NOT NULL DEFAULT 0,
 completed INTEGER NOT NULL DEFAULT 0,
 progress_session TEXT NOT NULL DEFAULT '',
 progress_sequence INTEGER NOT NULL DEFAULT 0,
 progress_event_ms INTEGER NOT NULL DEFAULT 0,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(profile_id,media_id)
);
CREATE TABLE profile_watch_daily (
 profile_id INTEGER NOT NULL,
 media_id INTEGER NOT NULL,
 watch_date TEXT NOT NULL,
 seconds_watched REAL NOT NULL DEFAULT 0,
 completed INTEGER NOT NULL DEFAULT 0,
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(profile_id,media_id,watch_date)
);`)
	if err != nil { t.Fatal(err) }

	s := &Service{db: db}
	ctx := context.Background()
	if ok, err := s.SaveProfileProgressOrdered(ctx, 1, 9, 300, 3600, "session-a", 1, 1000); err != nil || !ok {
		t.Fatalf("first update ok=%v err=%v", ok, err)
	}
	if ok, err := s.SaveProfileProgressOrdered(ctx, 1, 9, 1800, 3600, "session-a", 2, 2000); err != nil || !ok {
		t.Fatalf("seek update ok=%v err=%v", ok, err)
	}
	if ok, err := s.SaveProfileProgressOrdered(ctx, 1, 9, 300, 3600, "session-a", 1, 1500); err != nil || ok {
		t.Fatalf("stale request should be rejected: ok=%v err=%v", ok, err)
	}
	var position float64
	if err := db.QueryRow(`SELECT position_seconds FROM profile_progress WHERE profile_id=1 AND media_id=9`).Scan(&position); err != nil { t.Fatal(err) }
	if position != 1800 { t.Fatalf("stale request overwrote position: %.0f", position) }

	// A newer backwards seek is legitimate and must win because ordering is
	// temporal/session based, not based on the largest position value.
	if ok, err := s.SaveProfileProgressOrdered(ctx, 1, 9, 900, 3600, "session-a", 3, 3000); err != nil || !ok {
		t.Fatalf("backward seek ok=%v err=%v", ok, err)
	}
	if err := db.QueryRow(`SELECT position_seconds FROM profile_progress WHERE profile_id=1 AND media_id=9`).Scan(&position); err != nil { t.Fatal(err) }
	if position != 900 { t.Fatalf("backward seek was not persisted: %.0f", position) }
}
