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

func TestParseAnimeSeriesNoisyDubbedReleaseNames(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		title   string
		season  int
		episode int
	}{
		{
			name:    "InuYasha Memoria da TV",
			path:    "/media/akumanimes/Animes/Animes Dublado/Inuyasha/InuYasha.107.MemoriadaTV.BDMenor.mkv",
			title:   "Inuyasha",
			season:  1,
			episode: 107,
		},
		{
			name:    "Bucky upscale",
			path:    "/media/akumanimes/Animes/Animes Dublado/Bucky/Bucky.18.Upscale1080p.MemoriadaTV.Maior.mkv",
			title:   "Bucky",
			season:  1,
			episode: 18,
		},
		{
			name:    "Samurai X Epi glued release tail",
			path:    "/media/akumanimes/Animes/Animes Dublado/Samurai X/SamuraiX-Epi90UpsAI1080p.MemoriadaTV.Maior.mkv",
			title:   "Samurai X",
			season:  1,
			episode: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := ParseFilename(tt.path, "anime_series")
			if parsed.Title != tt.title {
				t.Fatalf("Title=%q want %q", parsed.Title, tt.title)
			}
			if parsed.Season != tt.season || parsed.Episode != tt.episode {
				t.Fatalf("season/episode=%d/%d want %d/%d", parsed.Season, parsed.Episode, tt.season, tt.episode)
			}
			if parsed.LikelyMovie {
				t.Fatal("dubbed anime episode must not be classified as movie")
			}
		})
	}
}
