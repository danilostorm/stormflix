package games

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverROMsSupportsInitialG1MatrixAndSidecar(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"Super Mario World (USA) [!].sfc": "snes-data",
		"Pokemon Emerald (USA).gba":       "gba-data",
		"Sonic.bin":                       "ignored",
		"notes.txt":                       "ignored",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "Pokemon Emerald (USA).jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}

	roms, err := discoverROMs(context.Background(), root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(roms) != 2 {
		t.Fatalf("expected 2 supported ROMs, got %d: %+v", len(roms), roms)
	}
	byPlatform := map[string]discoveredROM{}
	for _, rom := range roms {
		byPlatform[rom.Platform] = rom
	}
	if got := byPlatform["snes"].Title; got != "Super Mario World" {
		t.Fatalf("clean SNES title=%q", got)
	}
	if got := byPlatform["gba"].Title; got != "Pokemon Emerald" {
		t.Fatalf("clean GBA title=%q", got)
	}
	if byPlatform["gba"].CoverPath == "" {
		t.Fatal("expected local sidecar cover")
	}
}

func TestHashROMUsesSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.nes")
	body := []byte("stormflix-game-identity")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := hashROM(context.Background(), path, int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("hash=%s want=%s", got, want)
	}
}

func TestSupportedExtensionsStayCartridgeFocused(t *testing.T) {
	for _, ext := range []string{".nes", ".sfc", ".smc", ".md", ".gen", ".smd", ".gb", ".gbc", ".gba"} {
		if platformByExtension[ext] == "" {
			t.Fatalf("missing G1 extension %s", ext)
		}
	}
	if platformByExtension[".iso"] != "" || platformByExtension[".zip"] != "" {
		t.Fatal("disc/archive formats must not enter the G1 cartridge scanner")
	}
}