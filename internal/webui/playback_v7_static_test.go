package webui

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestPlaybackV7OriginalFileRuntimeIsAutomaticAndHidden(t *testing.T) {
	core, err := Static.ReadFile("static/playback-core-v53.js")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := Static.ReadFile("static/local-origin-player.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"original_file", "wasm_simd", "original_local_decode", "plan?.local_origin", "sfLocalOrigin.load", "localOriginRuntimeFailed=true"} {
		if !bytes.Contains(core, []byte(want)) {
			t.Fatalf("Playback v7 core missing %q", want)
		}
	}
	for _, want := range []string{"checkUseMSE:()=>false", "enableWebCodecs:true", "enableWorker:true", "preLoadTime:3", "credentials:'same-origin'", "engine.selectAudio", "engine.selectSubtitle", "engine.destroy"} {
		if !bytes.Contains(runtime, []byte(want)) {
			t.Fatalf("Playback v7 local runtime missing %q", want)
		}
	}
	combined := append(append([]byte{}, core...), runtime...)
	for _, forbidden := range []string{"setLocalOriginEnabled", "stormflix.player.local_origin", "data-local-origin-toggle"} {
		if bytes.Contains(combined, []byte(forbidden)) {
			t.Fatalf("local-origin policy must not expose user control %q", forbidden)
		}
	}
}

func TestPlaybackV7VendoredLibmediaChecksums(t *testing.T) {
	manifest, err := Static.ReadFile("static/vendor-libmedia/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) != 2 {
			t.Fatalf("invalid checksum row %q", scanner.Text())
		}
		body, readErr := Static.ReadFile("static/vendor-libmedia/" + parts[1])
		if readErr != nil {
			t.Fatalf("read %s: %v", parts[1], readErr)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != parts[0] {
			t.Fatalf("checksum mismatch for %s", parts[1])
		}
		checked++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if checked < 40 {
		t.Fatalf("expected complete pinned runtime, checked only %d assets", checked)
	}
}
