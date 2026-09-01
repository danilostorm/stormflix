package games

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func makeCoreBundle(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func TestCoreBundleURLUsesPinnedZIPArtifact(t *testing.T) {
	url := coreBundleURL("snes9x")
	if !strings.Contains(url, "@"+RetroArchBuild+"/retroarch/snes9x_libretro.zip") {
		t.Fatalf("unexpected core bundle URL: %s", url)
	}
}

func TestExtractCoreBundleFindsJSAndWASM(t *testing.T) {
	payload := makeCoreBundle(t, map[string][]byte{
		"snes9x_libretro.js":   []byte("console.log('snes9x')"),
		"snes9x_libretro.wasm": []byte{0x00, 0x61, 0x73, 0x6d},
		"snes9x_libretro.info": []byte("ignored"),
	})
	files, err := extractCoreBundle(payload)
	if err != nil {
		t.Fatalf("extract bundle: %v", err)
	}
	if string(files.JS) != "console.log('snes9x')" {
		t.Fatalf("unexpected JS payload: %q", files.JS)
	}
	if !bytes.Equal(files.WASM, []byte{0x00, 0x61, 0x73, 0x6d}) {
		t.Fatalf("unexpected WASM payload: %v", files.WASM)
	}
}

func TestExtractCoreBundleRejectsMissingRuntimePair(t *testing.T) {
	payload := makeCoreBundle(t, map[string][]byte{
		"snes9x_libretro.js": []byte("console.log('only js')"),
	})
	if _, err := extractCoreBundle(payload); err == nil {
		t.Fatal("expected bundle without WASM to be rejected")
	}
}
