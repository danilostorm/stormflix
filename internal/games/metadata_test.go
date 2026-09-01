package games

import "testing"

func TestGameMetadataTitleScorePrefersEquivalentNames(t *testing.T) {
	cases := []struct {
		a, b string
		min  float64
	}{
		{"Pokémon Emerald", "Pokemon Emerald", .99},
		{"Sonic the Hedgehog 2", "Sonic The Hedgehog 2", .99},
		{"Super Mario World (USA)", "Super Mario World", .70},
	}
	for _, tc := range cases {
		if got := titleScore(tc.a, tc.b); got < tc.min {
			t.Fatalf("titleScore(%q,%q)=%.3f want >= %.3f", tc.a, tc.b, got, tc.min)
		}
	}
	if got := titleScore("Metroid", "Sonic the Hedgehog"); got > .4 {
		t.Fatalf("unrelated titles scored too high: %.3f", got)
	}
}

func TestArtworkExtensionUsesContentTypeAndSafeFallback(t *testing.T) {
	if got := artworkExtension("https://example.invalid/art", "image/webp"); got != ".webp" {
		t.Fatalf("webp extension=%q", got)
	}
	if got := artworkExtension("https://example.invalid/cover.png?x=1", "application/octet-stream"); got != ".png" {
		t.Fatalf("url extension=%q", got)
	}
	if got := artworkExtension("https://example.invalid/noext", "application/octet-stream"); got != ".jpg" {
		t.Fatalf("fallback extension=%q", got)
	}
}
