package library

import "testing"

func TestSameOrInside(t *testing.T) {
	cases := []struct {
		path string
		root string
		want bool
	}{
		{"/media/Guilherme/Filmes 4K", "/media/Guilherme", true},
		{"/media/Guilherme", "/media/Guilherme", true},
		{"/media/Guilherme2", "/media/Guilherme", false},
		{"/media/Akumanimes/Animes", "/media/Guilherme", false},
	}
	for _, tc := range cases {
		if got := sameOrInside(tc.path, tc.root); got != tc.want {
			t.Fatalf("sameOrInside(%q,%q)=%v want %v", tc.path, tc.root, got, tc.want)
		}
	}
}
