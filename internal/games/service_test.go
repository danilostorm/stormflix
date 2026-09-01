package games

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func writeROMZip(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := entry.Write(body); err != nil {
			_ = writer.Close()
			_ = file.Close()
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

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

func TestDiscoverROMsSupportsSingleCartridgeInsideZIP(t *testing.T) {
	root := t.TempDir()
	body := []byte("true-lies-snes-rom")
	archive := filepath.Join(root, "True Lies (USA).zip")
	writeROMZip(t, archive, map[string][]byte{
		"True Lies (USA).sfc": body,
		"README.txt":         []byte("not a ROM"),
	})

	roms, err := discoverROMs(context.Background(), root)
	if err != nil {
		t.Fatalf("discover zip: %v", err)
	}
	if len(roms) != 1 {
		t.Fatalf("expected 1 zipped ROM, got %d: %+v", len(roms), roms)
	}
	rom := roms[0]
	if rom.Platform != "snes" {
		t.Fatalf("platform=%q want snes", rom.Platform)
	}
	if rom.Extension != ".zip" {
		t.Fatalf("extension=%q want .zip", rom.Extension)
	}
	if rom.Title != "True Lies" {
		t.Fatalf("title=%q want True Lies", rom.Title)
	}
	if rom.SizeBytes != int64(len(body)) {
		t.Fatalf("uncompressed size=%d want %d", rom.SizeBytes, len(body))
	}
}

func TestDiscoverROMsDetectsZIPPlatformFromInnerROM(t *testing.T) {
	root := t.TempDir()
	writeROMZip(t, filepath.Join(root, "Metroid.zip"), map[string][]byte{"Metroid.nes": []byte("nes-rom")})
	writeROMZip(t, filepath.Join(root, "Pokemon.zip"), map[string][]byte{"Pokemon.gba": []byte("gba-rom")})

	roms, err := discoverROMs(context.Background(), root)
	if err != nil {
		t.Fatalf("discover zips: %v", err)
	}
	if len(roms) != 2 {
		t.Fatalf("expected 2 zipped ROMs, got %d: %+v", len(roms), roms)
	}
	seen := map[string]bool{}
	for _, rom := range roms {
		seen[rom.Platform] = true
	}
	if !seen["nes"] || !seen["gba"] {
		t.Fatalf("inner extension platform detection failed: %+v", roms)
	}
}

func TestDiscoverROMsIgnoresAmbiguousOrUnsupportedZIP(t *testing.T) {
	root := t.TempDir()
	writeROMZip(t, filepath.Join(root, "ambiguous.zip"), map[string][]byte{
		"one.sfc": []byte("one"),
		"two.smc": []byte("two"),
	})
	writeROMZip(t, filepath.Join(root, "unsupported.zip"), map[string][]byte{
		"disc.iso": []byte("disc"),
	})

	roms, err := discoverROMs(context.Background(), root)
	if err != nil {
		t.Fatalf("discover invalid zips: %v", err)
	}
	if len(roms) != 0 {
		t.Fatalf("ambiguous/unsupported ZIPs must not enter catalog: %+v", roms)
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

func TestHashROMZIPUsesUncompressedROMIdentity(t *testing.T) {
	root := t.TempDir()
	body := []byte("same-rom-content-zipped-or-loose")
	zipPath := filepath.Join(root, "game.zip")
	loosePath := filepath.Join(root, "game.sfc")
	writeROMZip(t, zipPath, map[string][]byte{"game.sfc": body})
	if err := os.WriteFile(loosePath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	zippedHash, err := hashROM(context.Background(), zipPath, int64(len(body)))
	if err != nil {
		t.Fatalf("hash zipped ROM: %v", err)
	}
	looseHash, err := hashROM(context.Background(), loosePath, int64(len(body)))
	if err != nil {
		t.Fatalf("hash loose ROM: %v", err)
	}
	if zippedHash != looseHash {
		t.Fatalf("zipped hash=%s loose hash=%s; identity must use uncompressed ROM", zippedHash, looseHash)
	}
}

func TestSupportedExtensionsStayCartridgeFocusedWithSafeZIPContainer(t *testing.T) {
	for _, ext := range []string{".nes", ".sfc", ".smc", ".md", ".gen", ".smd", ".gb", ".gbc", ".gba"} {
		if platformByExtension[ext] == "" {
			t.Fatalf("missing G1 extension %s", ext)
		}
	}
	if platformByExtension[".iso"] != "" {
		t.Fatal("disc formats must not enter the cartridge scanner")
	}
	if platformByExtension[".zip"] != "" {
		t.Fatal("ZIP platform must be detected from its single supported inner ROM")
	}
	foundZIP := false
	for _, ext := range SupportedExtensions() {
		if ext == ".zip" {
			foundZIP = true
			break
		}
	}
	if !foundZIP {
		t.Fatal("safe single-ROM ZIP container must be advertised as supported")
	}
}
