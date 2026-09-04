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
		"/vendor-hevc-hls-plugin-0.1.2.js",
		"/vendor-hevc-transcode-worker-1.4.2.js",
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

func TestPlaybackV6LocalDecodeClientSafety(t *testing.T) {
	core, err := os.ReadFile("static/playback-core-v53.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(core)
	for _, want := range []string{
		"function localDecodeClientKind()",
		"window.NativePlaybackAnywhere",
		"return'android_webview'",
		"return'mobile_web'",
		"return'tv'",
		"navigator.userAgentData?.mobile",
		"navigator.maxTouchPoints",
		"kind==='web'",
		"const automatic4K=cores>=12&&(memory===0||memory>=8)",
		"if(automatic4K)maxHeight=2160",
		"const enabled=!localDecodeRuntimeFailed&&kind==='web'",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 client safety missing %q", want)
		}
	}
	for _, forbidden := range []string{"stormflix.player.local_decode", "setLocalDecodeEnabled"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Playback v6 automatic policy must not expose stored user control %q", forbidden)
		}
	}
}

func TestPlaybackV6PlayerExposesLocalDecodeStatus(t *testing.T) {
	player, err := os.ReadFile("static/player-v5.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(player)
	for _, want := range []string{"WASM LOCAL DECODE", "Playback Engine v6", "sfLocalDecodeStats"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 player missing %q", want)
		}
	}
	for _, forbidden := range []string{"data-v6-local-toggle", "sf-v6-local-row", "Decode local ativado", "setLocalDecodeEnabled"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Playback v6 player must keep local-decode policy internal; found %q", forbidden)
		}
	}
}

func TestPlaybackV6AdminExplainsAdaptiveOrder(t *testing.T) {
	admin, err := os.ReadFile("static/admin/admin-transcode.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(admin)
	for _, want := range []string{
		"Playback Engine v6",
		"Direct Play",
		"WASM",
		"servidor transcodifica vídeo somente",
		"function localDecodeClientKind()",
		"navigator.userAgentData?.mobile",
		"navigator.maxTouchPoints",
		"local.desktop&&local.secure&&local.webcodecs&&local.wasm",
		"AUTOMÁTICO",
		"Limite local automático",
		"A rota é automática e não possui controle de usuário",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Playback v6 admin missing %q", want)
		}
	}
	for _, forbidden := range []string{"data-v6-local", "stormflix.player.local_decode", "4K local (opt-in)"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Playback v6 Admin must be diagnostic-only; found user control %q", forbidden)
		}
	}
}
