package games

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestGamePreviewSaveIsVersionedAndProfileOwned(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	if MaxSaveBytes("preview") != 2<<20 {
		t.Fatalf("preview size limit=%d", MaxSaveBytes("preview"))
	}
	first, err := svc.WriteSave(ctx, profileID, gameID, "preview", 0, []byte("preview-one"))
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}
	second, err := svc.WriteSave(ctx, profileID, gameID, "preview", 0, []byte("preview-two"))
	if err != nil {
		t.Fatalf("second preview: %v", err)
	}
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("preview versions=%d,%d", first.Version, second.Version)
	}
	file, err := svc.SaveFile(ctx, profileID, gameID, "preview", 0)
	if err != nil {
		t.Fatalf("preview file: %v", err)
	}
	if !strings.HasSuffix(file.Path, "preview-0.webp") {
		t.Fatalf("unexpected preview path: %s", file.Path)
	}
	body, err := os.ReadFile(file.Path)
	if err != nil || string(body) != "preview-two" {
		t.Fatalf("preview body=%q err=%v", body, err)
	}
	backup, err := os.ReadFile(file.Path + ".bak1")
	if err != nil || string(backup) != "preview-one" {
		t.Fatalf("preview backup=%q err=%v", backup, err)
	}
}

func TestGamePreviewSaveDoesNotCrossProfiles(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	if _, err := svc.db.Exec(`INSERT INTO users(username,display_name,password_hash) VALUES('preview2','Preview 2','test')`); err != nil {
		t.Fatalf("insert second user: %v", err)
	}
	var secondProfile int64
	if err := svc.db.QueryRow(`SELECT p.id FROM profiles p JOIN users u ON u.id=p.user_id WHERE u.username='preview2'`).Scan(&secondProfile); err != nil {
		t.Fatalf("second profile: %v", err)
	}
	ctx := context.Background()
	if _, err := svc.WriteSave(ctx, profileID, gameID, "preview", 0, []byte("one")); err != nil {
		t.Fatalf("profile one preview: %v", err)
	}
	if _, err := svc.WriteSave(ctx, secondProfile, gameID, "preview", 0, []byte("two")); err != nil {
		t.Fatalf("profile two preview: %v", err)
	}
	one, _ := svc.SaveFile(ctx, profileID, gameID, "preview", 0)
	two, _ := svc.SaveFile(ctx, secondProfile, gameID, "preview", 0)
	if one.Path == two.Path {
		t.Fatal("profiles unexpectedly share preview path")
	}
}
