package settings

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/database"
)

func TestTraktApplicationSettingsAreEncryptedAndApplied(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	service, err := New(db, dir)
	if err != nil {
		t.Fatalf("settings service: %v", err)
	}
	clientID := "client-id-plain"
	clientSecret := "client-secret-plain"
	redirect := "urn:ietf:wg:oauth:2.0:oob"
	if err := service.UpdateTraktApplication(context.Background(), TraktApplicationUpdate{
		ClientID: &clientID, ClientSecret: &clientSecret, RedirectURI: &redirect,
	}); err != nil {
		t.Fatalf("update Trakt application: %v", err)
	}

	var storedID, storedSecret string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='trakt_client_id'`).Scan(&storedID); err != nil {
		t.Fatalf("read stored client id: %v", err)
	}
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='trakt_client_secret'`).Scan(&storedSecret); err != nil {
		t.Fatalf("read stored client secret: %v", err)
	}
	if !strings.HasPrefix(storedID, "enc:v1:") || !strings.HasPrefix(storedSecret, "enc:v1:") {
		t.Fatalf("Trakt application credentials are not encrypted at rest")
	}
	if strings.Contains(storedID, clientID) || strings.Contains(storedSecret, clientSecret) {
		t.Fatalf("plain Trakt credentials leaked into settings table")
	}

	gotID, gotSecret, gotRedirect, public, err := service.TraktApplication(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("resolve Trakt application: %v", err)
	}
	if gotID != clientID || gotSecret != clientSecret || gotRedirect != redirect {
		t.Fatalf("unexpected resolved settings id=%q secret=%q redirect=%q", gotID, gotSecret, gotRedirect)
	}
	if !public.Configured || !public.ClientIDConfigured || !public.ClientSecretConfigured {
		t.Fatalf("unexpected public Trakt status: %+v", public)
	}
}
