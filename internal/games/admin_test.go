package games

import (
	"context"
	"testing"
)

func TestProviderSettingsNeverExposeSecrets(t *testing.T) {
	svc, _, _, _ := seedGameG2(t)
	ctx := context.Background()
	if err := svc.UpdateProviderSettings(ctx, ProviderUpdate{Provider: "igdb", Enabled: true, Values: map[string]string{"client_id": "public-id", "client_secret": "very-secret"}}); err != nil {
		t.Fatalf("update provider: %v", err)
	}
	providers, err := svc.ProviderSettings(ctx)
	if err != nil {
		t.Fatalf("providers: %v", err)
	}
	var igdb ProviderInfo
	for _, provider := range providers {
		if provider.Key == "igdb" {
			igdb = provider
			break
		}
	}
	if !igdb.Enabled || !igdb.Configured || igdb.Public["client_id"] != "public-id" || !igdb.Secrets["client_secret"] {
		t.Fatalf("unexpected public provider state: %+v", igdb)
	}
	for _, value := range igdb.Public {
		if value == "very-secret" {
			t.Fatal("secret leaked through public provider response")
		}
	}
	pub, secrets, enabled, err := svc.ProviderSecretsForRuntime(ctx, "igdb")
	if err != nil || !enabled || pub["client_id"] != "public-id" || secrets["client_secret"] != "very-secret" {
		t.Fatalf("runtime provider values not recoverable: enabled=%v pub=%v secrets=%v err=%v", enabled, pub, secrets, err)
	}
}

func TestSaveGalleryIsProfileScoped(t *testing.T) {
	svc, profileID, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	if _, err := svc.WriteSave(ctx, profileID, gameID, "state", 0, []byte("profile-one")); err != nil {
		t.Fatalf("save profile one: %v", err)
	}
	items, err := svc.SaveGallery(ctx, profileID, nil, 20)
	if err != nil || len(items) != 1 || items[0].GameID != gameID || items[0].Kind != "state" {
		t.Fatalf("profile save gallery=%+v err=%v", items, err)
	}
	if items, err := svc.SaveGallery(ctx, profileID+999, nil, 20); err != nil || len(items) != 0 {
		t.Fatalf("unrelated profile should not see saves: %+v err=%v", items, err)
	}
}
