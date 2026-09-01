package games

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataProviderOverridesPromoteWiredProviders(t *testing.T) {
	want := map[string]bool{
		"igdb": true, "mobygames": true, "steamgriddb": true,
		"screenscraper": true, "retroachievements": true, "hasheous": true,
		"thegamesdb": true, "libretro": true,
	}
	for _, definition := range providerDefinitions {
		if want[definition.Key] {
			if definition.Stage != "configuravel" || !metadataProviderRuntimeSupported(definition.Key) {
				t.Fatalf("provider %s is not wired/configurable: stage=%s", definition.Key, definition.Stage)
			}
		}
	}
	for _, key := range []string{"playmatch", "launchbox", "flashpoint", "hltb", "demozoo", "pouet", "csdb"} {
		definition, ok := providerDefinitionFor(key)
		if !ok {
			t.Fatalf("missing roadmap provider %s", key)
		}
		if definition.Stage != "planejado" || metadataProviderRuntimeSupported(key) {
			t.Fatalf("roadmap provider %s must not be exposed as implemented: stage=%s", key, definition.Stage)
		}
	}
}

func TestHasheousDefaultURLDoesNotNeedToBeStored(t *testing.T) {
	svc, _, _, _ := seedGameG2(t)
	ctx := context.Background()
	if err := svc.UpdateProviderSettings(ctx, ProviderUpdate{
		Provider: "hasheous",
		Enabled:  true,
		Values:   map[string]string{"api_key": "client-key"},
	}); err != nil {
		t.Fatalf("update hasheous: %v", err)
	}
	providers, err := svc.ProviderSettings(ctx)
	if err != nil {
		t.Fatalf("provider settings: %v", err)
	}
	for _, provider := range providers {
		if provider.Key == "hasheous" {
			if !provider.Enabled || !provider.Configured || !provider.Secrets["api_key"] {
				t.Fatalf("unexpected Hasheous state: %+v", provider)
			}
			return
		}
	}
	t.Fatal("Hasheous provider missing")
}

func TestLibretroCanBeEnabledWithoutCredentials(t *testing.T) {
	svc, _, _, _ := seedGameG2(t)
	ctx := context.Background()
	if err := svc.UpdateProviderSettings(ctx, ProviderUpdate{Provider: "libretro", Enabled: true}); err != nil {
		t.Fatalf("enable libretro: %v", err)
	}
	providers, err := svc.ProviderSettings(ctx)
	if err != nil {
		t.Fatalf("provider settings: %v", err)
	}
	for _, provider := range providers {
		if provider.Key == "libretro" {
			if !provider.Enabled || !provider.Configured || len(provider.Fields) != 0 {
				t.Fatalf("unexpected Libretro state: %+v", provider)
			}
			return
		}
	}
	t.Fatal("Libretro provider missing")
}

func TestPersistProviderIDsKeepsCrossProviderIdentity(t *testing.T) {
	svc, _, gameID, _ := seedGameG2(t)
	ctx := context.Background()
	ids := map[string]string{
		"igdb": "123", "screenscraper": "456", "retroachievements": "789",
		"hasheous": "42", "thegamesdb": "99", "steamgriddb": "77", "libretro": "pokemon-emerald",
	}
	if err := svc.persistProviderIDs(ctx, gameID, ids); err != nil {
		t.Fatalf("persist ids: %v", err)
	}
	var igdb, ss, ra, hasheous, tgdb, sgdb, libretro string
	if err := svc.db.QueryRow(`SELECT igdb_id,screenscraper_id,retroachievements_id,hasheous_id,thegamesdb_id,steamgriddb_id,libretro_id FROM game_metadata WHERE game_id=?`, gameID).
		Scan(&igdb, &ss, &ra, &hasheous, &tgdb, &sgdb, &libretro); err != nil {
		t.Fatalf("read ids: %v", err)
	}
	if igdb != "123" || ss != "456" || ra != "789" || hasheous != "42" || tgdb != "99" || sgdb != "77" || libretro != "pokemon-emerald" {
		t.Fatalf("unexpected ids: igdb=%s ss=%s ra=%s hasheous=%s tgdb=%s sgdb=%s libretro=%s", igdb, ss, ra, hasheous, tgdb, sgdb, libretro)
	}
}

func TestGameLookupHashesRawROM(t *testing.T) {
	svc, _, gameID, _ := seedGameG2(t)
	body := []byte("stormflix-metadata-hash-fixture")
	romPath := filepath.Join(t.TempDir(), "fixture.gba")
	if err := os.WriteFile(romPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(romPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO game_files(game_id,path,extension,size_bytes,modified_unix,available) VALUES(?,?,?,?,?,1)`, gameID, romPath, ".gba", len(body), info.ModTime().Unix()); err != nil {
		t.Fatalf("insert game file: %v", err)
	}
	got, err := svc.gameLookupHashes(context.Background(), gameID)
	if err != nil {
		t.Fatalf("hash ROM: %v", err)
	}
	md5sum := md5.Sum(body)
	sha1sum := sha1.Sum(body)
	wantMD5 := hex.EncodeToString(md5sum[:])
	wantSHA1 := hex.EncodeToString(sha1sum[:])
	wantCRC := fmt.Sprintf("%08X", crc32.ChecksumIEEE(body))
	if got.MD5 != wantMD5 || got.SHA1 != wantSHA1 || got.CRC != wantCRC {
		t.Fatalf("hash mismatch: got=%+v want md5=%s sha1=%s crc=%s", got, wantMD5, wantSHA1, wantCRC)
	}
}

func TestExplicitRetroAchievementsID(t *testing.T) {
	for input, want := range map[string]string{
		"Pokemon Emerald (ra-11240)": "11240",
		"Game (RA-99)":               "99",
		"Game (ra-nope)":             "",
		"Game":                       "",
	} {
		if got := explicitRAID(input); got != want {
			t.Fatalf("explicitRAID(%q)=%q want %q", input, got, want)
		}
	}
}
