package games

import "testing"

func TestPublicImageURLsFiltersUnsafeSchemes(t *testing.T) {
	got := publicImageURLs([]string{
		"https://images.example/game-1.jpg",
		"javascript:alert(1)",
		"file:///tmp/game.png",
		"http://cdn.example/game-2.png",
		"https://images.example/game-1.jpg",
	}, 3)
	if len(got) != 3 {
		t.Fatalf("got %d public URLs, want 3: %#v", len(got), got)
	}
	if got[0] != "https://images.example/game-1.jpg" || got[1] != "http://cdn.example/game-2.png" {
		t.Fatalf("unexpected filtered URLs: %#v", got)
	}
}

func TestDecodeStringListIgnoresEmptyAndDuplicateValues(t *testing.T) {
	got := decodeStringList(`["Action"," Action ","","Arcade"]`)
	if len(got) != 2 || got[0] != "Action" || got[1] != "Arcade" {
		t.Fatalf("unexpected decoded values: %#v", got)
	}
}
