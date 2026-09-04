package assets

import "testing"

func TestResponsiveVariantPolicy(t *testing.T) {
	widths := map[int]int{-1: 0, 0: 0, 1: 240, 241: 360, 500: 500, 501: 780, 2000: 1280}
	for input, wanted := range widths {
		if got := responsiveWidth(input); got != wanted {
			t.Fatalf("responsiveWidth(%d)=%d, want %d", input, got, wanted)
		}
	}
	if got := responsiveFormat("image/avif,image/webp,*/*"); got != "avif" {
		t.Fatalf("format=%q, want avif", got)
	}
	if got := responsiveFormat("image/webp,*/*"); got != "webp" {
		t.Fatalf("format=%q, want webp", got)
	}
	if got := responsiveFormat("image/jpeg"); got != "" {
		t.Fatalf("format=%q, want source fallback", got)
	}
}
