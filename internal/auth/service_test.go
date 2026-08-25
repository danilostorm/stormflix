package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/database"
)

func TestListUsersDoesNotDeadlock(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix-test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	svc := auth.NewService(db)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := svc.CreateFirstAdmin(ctx, "admin", "Administrator", "password123"); err != nil {
		t.Fatalf("create first admin: %v", err)
	}

	users, err := svc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "admin" {
		t.Fatalf("expected admin user, got %q", users[0].Username)
	}
}
