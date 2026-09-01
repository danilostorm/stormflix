package games

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danilostorm/stormflix/internal/database"
)

func seedGameG2(t *testing.T) (*Service, int64, int64, string) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := database.Open(filepath.Join(dataDir, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	libraryResult, err := db.Exec(`INSERT INTO libraries(name,kind,path) VALUES('Jogos','games','/tmp/stormflix-games-test')`)
	if err != nil {
		t.Fatalf("insert library: %v", err)
	}
	libraryID, _ := libraryResult.LastInsertId()
	if _, err := db.Exec(`INSERT INTO users(username,display_name,password_hash) VALUES('player','Player','test')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var profileID int64
	if err := db.QueryRow(`SELECT p.id FROM profiles p JOIN users u ON u.id=p.user_id WHERE u.username='player'`).Scan(&profileID); err != nil {
		t.Fatalf("profile: %v", err)
	}
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	gameResult, err := db.Exec(`INSERT INTO games(library_id,platform,title,sort_title,content_hash) VALUES(?,?,?,?,?)`, libraryID, "gba", "Pokemon Emerald", "pokemon emerald", hash)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	gameID, _ := gameResult.LastInsertId()
	return NewService(db), profileID, gameID, dataDir
}

func TestWriteSaveIsAtomicVersionedAndRecoverable(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	first, err := svc.WriteSave(ctx, profileID, gameID, "state", 0, []byte("state-one"))
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("first version=%d", first.Version)
	}
	second, err := svc.WriteSave(ctx, profileID, gameID, "state", 0, []byte("state-two"))
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if second.Version != 2 {
		t.Fatalf("second version=%d", second.Version)
	}
	file, err := svc.SaveFile(ctx, profileID, gameID, "state", 0)
	if err != nil {
		t.Fatalf("save file: %v", err)
	}
	body, err := os.ReadFile(file.Path)
	if err != nil || string(body) != "state-two" {
		t.Fatalf("current save=%q err=%v", body, err)
	}
	backup, err := os.ReadFile(file.Path + ".bak1")
	if err != nil || string(backup) != "state-one" {
		t.Fatalf("backup save=%q err=%v", backup, err)
	}
}

func TestGameSavesAreIsolatedByProfile(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	if _, err := svc.db.Exec(`INSERT INTO users(username,display_name,password_hash) VALUES('player2','Player 2','test')`); err != nil {
		t.Fatalf("insert second user: %v", err)
	}
	var secondProfile int64
	if err := svc.db.QueryRow(`SELECT p.id FROM profiles p JOIN users u ON u.id=p.user_id WHERE u.username='player2'`).Scan(&secondProfile); err != nil {
		t.Fatalf("second profile: %v", err)
	}
	if _, err := svc.WriteSave(ctx, profileID, gameID, "sram", 0, []byte("profile-one")); err != nil {
		t.Fatalf("profile one save: %v", err)
	}
	if _, err := svc.WriteSave(ctx, secondProfile, gameID, "sram", 0, []byte("profile-two")); err != nil {
		t.Fatalf("profile two save: %v", err)
	}
	one, err := svc.SaveFile(ctx, profileID, gameID, "sram", 0)
	if err != nil {
		t.Fatalf("profile one file: %v", err)
	}
	two, err := svc.SaveFile(ctx, secondProfile, gameID, "sram", 0)
	if err != nil {
		t.Fatalf("profile two file: %v", err)
	}
	if one.Path == two.Path {
		t.Fatal("profiles unexpectedly share the same save path")
	}
	oneBody, _ := os.ReadFile(one.Path)
	twoBody, _ := os.ReadFile(two.Path)
	if string(oneBody) != "profile-one" || string(twoBody) != "profile-two" {
		t.Fatalf("isolated saves changed: one=%q two=%q", oneBody, twoBody)
	}
}

func TestGamePlayHeartbeatCreditsOnlyMonotonicDelta(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	session := "stormflix-game-session-123456789"
	if total, err := svc.Heartbeat(ctx, profileID, gameID, session, 0); err != nil || total != 0 {
		t.Fatalf("start total=%d err=%v", total, err)
	}
	if total, err := svc.Heartbeat(ctx, profileID, gameID, session, 15); err != nil || total != 15 {
		t.Fatalf("15s total=%d err=%v", total, err)
	}
	if total, err := svc.Heartbeat(ctx, profileID, gameID, session, 40); err != nil || total != 40 {
		t.Fatalf("40s total=%d err=%v", total, err)
	}
	// A suspended/forged large jump is bounded to 120 seconds for one update.
	if total, err := svc.Heartbeat(ctx, profileID, gameID, session, 1000); err != nil || total != 160 {
		t.Fatalf("bounded jump total=%d err=%v", total, err)
	}
}

func TestGameScanYieldsToActiveGameSession(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	if _, err := svc.Heartbeat(context.Background(), profileID, gameID, "stormflix-active-game-123456789", 0); err != nil {
		t.Fatalf("seed active game session: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if svc.waitForPlayback(ctx, 0, 0, 1, 0, 0) {
		t.Fatal("game scan must yield while a game session is active")
	}
}
