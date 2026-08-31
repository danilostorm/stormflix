package httpapi

import (
	"path/filepath"
	"testing"
)

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

func TestNormalizedFilesystemRootsIncludesManagedRootsAndDeduplicates(t *testing.T) {
	media := filepath.Clean("/media")
	alien := filepath.Clean("/mnt/remotes/ALIEN_Filmes")
	local := filepath.Clean("/mnt/user/Filmes")
	roots := normalizedFilesystemRoots(media, []string{alien, local, alien, " "})
	if len(roots) != 3 {
		t.Fatalf("len(roots) = %d, want 3: %#v", len(roots), roots)
	}
	want := []string{media, alien, local}
	for i, path := range want {
		if roots[i].Path != path {
			t.Fatalf("roots[%d].Path = %q, want %q", i, roots[i].Path, path)
		}
	}
}

func TestFilesystemRootForPathAllowsOnlyAuthorizedRoots(t *testing.T) {
	roots := normalizedFilesystemRoots("/media", []string{"/mnt/remotes/ALIEN_Filmes", "/mnt/user/Filmes"})
	cases := []struct {
		path string
		root string
		ok   bool
	}{
		{"/media/akumanimes", "/media", true},
		{"/mnt/remotes/ALIEN_Filmes/Filme", "/mnt/remotes/ALIEN_Filmes", true},
		{"/mnt/user/Filmes/Filme", "/mnt/user/Filmes", true},
		{"/etc", "", false},
		{"/mnt/user", "", false},
	}
	for _, tc := range cases {
		root, ok := filesystemRootForPath(roots, tc.path)
		if ok != tc.ok {
			t.Fatalf("filesystemRootForPath(%q) ok = %v, want %v", tc.path, ok, tc.ok)
		}
		if root.Path != tc.root {
			t.Fatalf("filesystemRootForPath(%q).Path = %q, want %q", tc.path, root.Path, tc.root)
		}
	}
}

func TestFilesystemRootForPathPrefersMostSpecificNestedRoot(t *testing.T) {
	roots := normalizedFilesystemRoots("/media", []string{"/media/Filmes"})
	root, ok := filesystemRootForPath(roots, "/media/Filmes/Classicos")
	if !ok {
		t.Fatal("expected path to be authorized")
	}
	if root.Path != "/media/Filmes" {
		t.Fatalf("root.Path = %q, want /media/Filmes", root.Path)
	}
}
