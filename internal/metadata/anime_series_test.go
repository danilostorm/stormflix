package metadata

import "testing"

func TestParseAnimeSeriesNumberedEpisodeInSeasonFolder(t *testing.T) {
	parsed := ParseFilename("/media/akumanimes/Animes/Animes Dublado/Naruto/Temporada 02/01 - Chegada.mkv", "anime_series")
	if parsed.Title != "Naruto" {
		t.Fatalf("Title=%q want Naruto", parsed.Title)
	}
	if parsed.Season != 2 {
		t.Fatalf("Season=%d want 2", parsed.Season)
	}
	if parsed.Episode != 1 {
		t.Fatalf("Episode=%d want 1", parsed.Episode)
	}
	if parsed.LikelyMovie {
		t.Fatal("anime_series episode must not be classified as movie")
	}
}

func TestParseAnimeSeriesSxxExx(t *testing.T) {
	parsed := ParseFilename("/media/Animes Dublados/Dragon Ball Super/Dragon.Ball.Super.S01E15.1080p.mkv", "anime_series")
	if parsed.Title != "Dragon Ball Super" {
		t.Fatalf("Title=%q want Dragon Ball Super", parsed.Title)
	}
	if parsed.Season != 1 || parsed.Episode != 15 {
		t.Fatalf("season/episode=%d/%d want 1/15", parsed.Season, parsed.Episode)
	}
}
