package webui

import (
	"strings"
	"testing"
)

func TestJellyfinAndroidWebViewBridgeIsEmbedded(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(index), "/main.stormflix.bundle.js") {
		t.Fatal("index.html does not request the Jellyfin Android handshake bundle")
	}
	if !strings.Contains(string(index), "window.NativeInterface") {
		t.Fatal("Jellyfin Android handshake must remain limited to the native WebView")
	}

	bundle, err := Static.ReadFile("static/main.stormflix.bundle.js")
	if err != nil {
		t.Fatalf("handshake bundle is not embedded: %v", err)
	}
	text := string(bundle)
	for _, required := range []string{
		"StormFlixJellyfinWebViewBridge",
		"jellyfin_credentials",
		"/api/v1/compat/jellyfin-mobile-bridge",
		"/Sessions/Capabilities/Full",
		"X-Emby-Token",
		"window.NativeInterface",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Jellyfin Android bridge is missing %q", required)
		}
	}
}
