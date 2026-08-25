package httpapi

import "testing"

func TestPathWithinRoot(t *testing.T) {
	root := "/media"
	cases := []struct {
		path string
		want bool
	}{
		{"/media", true},
		{"/media/Filmes", true},
		{"/media/Series/Temporada 01", true},
		{"/media2", false},
		{"/etc", false},
		{"/media/../etc", false},
	}
	for _, tc := range cases {
		if got := pathWithinRoot(root, tc.path); got != tc.want {
			t.Fatalf("pathWithinRoot(%q, %q) = %v, want %v", root, tc.path, got, tc.want)
		}
	}
}
