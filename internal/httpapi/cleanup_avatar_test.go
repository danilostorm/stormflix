package httpapi

import "testing"

func TestLocalAssetKeyFromURLPreservesProfileAvatars(t *testing.T) {
	cases := []struct {
		value string
		base  string
		want  string
	}{
		{"/assets/avatars/1/profile-2/avatar.webp", "", "avatars/1/profile-2/avatar.webp"},
		{"https://cdn.example.test/stormflix/avatars/1/profile-2/avatar.png", "https://cdn.example.test/stormflix", "avatars/1/profile-2/avatar.png"},
		{"https://example.org/avatar.png", "https://cdn.example.test/stormflix", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := localAssetKeyFromURL(tc.value, tc.base); got != tc.want {
			t.Fatalf("localAssetKeyFromURL(%q,%q)=%q want %q", tc.value, tc.base, got, tc.want)
		}
	}
}
