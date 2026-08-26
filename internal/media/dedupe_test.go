package media

import "testing"

func TestDedupeItemsCollapsesSameTMDBMovie(t *testing.T) {
	items := []Item{
		{ID: 10, Title: "A Era do Gelo: As Aventuras de Buck", MediaType: "movie", Year: 2022, TMDBID: 774825, PosterURL: "/poster.jpg", MetadataStatus: "matched"},
		{ID: 20, Title: "A Era do Gelo: As Aventuras de Buck", MediaType: "movie", Year: 2022, TMDBID: 774825, PosterURL: "/poster2.jpg", MetadataStatus: "matched"},
	}
	got := DedupeItems(items)
	if len(got) != 1 {
		t.Fatalf("expected one logical title, got %d", len(got))
	}
	if got[0].ID != 10 {
		t.Fatalf("expected stable first source as representative, got id %d", got[0].ID)
	}
}

func TestDedupeItemsKeepsDifferentEpisodes(t *testing.T) {
	items := []Item{
		{ID: 1, Title: "Episode 1", MediaType: "series", TMDBID: 123, SeasonNumber: 1, EpisodeNumber: 1},
		{ID: 2, Title: "Episode 2", MediaType: "series", TMDBID: 123, SeasonNumber: 1, EpisodeNumber: 2},
		{ID: 3, Title: "Episode 1 copy", MediaType: "series", TMDBID: 123, SeasonNumber: 1, EpisodeNumber: 1},
	}
	got := DedupeItems(items)
	if len(got) != 2 {
		t.Fatalf("expected two logical episodes, got %d", len(got))
	}
}

func TestDedupeItemsFallsBackToLocalizedTitleAndYear(t *testing.T) {
	items := []Item{
		{ID: 1, Title: "Belle", MediaType: "movie", Year: 2013, PosterURL: "/a.jpg"},
		{ID: 2, Title: "BELLE!", MediaType: "movie", Year: 2013, PosterURL: "/b.jpg"},
	}
	got := DedupeItems(items)
	if len(got) != 1 {
		t.Fatalf("expected punctuation/case variants to collapse, got %d", len(got))
	}
}
