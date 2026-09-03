package webui

import (
	"os"
	"strings"
	"testing"
)

func TestPlaybackV6LocalDecodeWiring(t *testing.T) {
	core, err := os.ReadFile("static/playback-core-v53.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(core)
	for _, want := range []string{
		"@hevcjs/hlsjs-plugin@0.1.2",
		"@hevcjs/core@1.4.2",
		"local_decode:browserLocalDecodeCapabilities()",
		"hevc_wasm",
		"localDecodeRuntimeFailed=true",
		"stormflix:local-decode-stat",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 core missing %q", want)
		}
	}
}

func TestPlaybackV6PlayerExposesLocalDecodeStatus(t *testing.T) {
	player, err := os.ReadFile("static/player-v5.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(player)
	for _, want := range []string{"WASM LOCAL DECODE", "Playback Engine v6", "data-v6-local-toggle", "sfLocalDecodeStats"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 player missing %q", want)
		}
	}
}

func TestPlaybackV6AdminExplainsAdaptiveOrder(t *testing.T) {
	admin, err := os.ReadFile("static/admin/admin-transcode.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(admin)
	for _, want := range []string{"Playback Engine v6", "Direct Play", "WASM", "servidor transcodifica vídeo somente"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 admin missing %q", want)
		}
	}
}
