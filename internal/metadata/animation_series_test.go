package metadata

import "testing"

func TestParseAnimationSeriesCompactEpisodeInsideRemuxFolder(t *testing.T) {
	parsed := ParseFilename("/media/akumanimes/Desenhos/Pica-Pau e seus Amigos/Remux/002PP-BD1080pRemux.mkv", "animation_series")
	if parsed.Title != "Pica-Pau e seus Amigos" {
		t.Fatalf("Title=%q want %q", parsed.Title, "Pica-Pau e seus Amigos")
	}
	if parsed.Season != 1 || parsed.Episode != 2 {
		t.Fatalf("season/episode=%d/%d want 1/2", parsed.Season, parsed.Episode)
	}
	if parsed.LikelyMovie {
		t.Fatal("cartoon episode must not be classified as movie")
	}
}

func TestParseAnimationSeriesExplicitSeasonFolder(t *testing.T) {
	parsed := ParseFilename("/media/Desenhos/Batman - A Série Animada/Temporada 02/07 - Episodio.mkv", "animation_series")
	if parsed.Title != "Batman - A Série Animada" {
		t.Fatalf("Title=%q want Batman - A Série Animada", parsed.Title)
	}
	if parsed.Season != 2 || parsed.Episode != 7 {
		t.Fatalf("season/episode=%d/%d want 2/7", parsed.Season, parsed.Episode)
	}
}

func TestParseAnimationSeriesLooseBrazilianSeasonFolder(t *testing.T) {
	parsed := ParseFilename("/media/Desenhos/X-Men 97/5ª Temporada - Stormbrasil/03XM-BD1080pRemux.mkv", "animation_series")
	if parsed.Title != "X-Men 97" {
		t.Fatalf("Title=%q want X-Men 97", parsed.Title)
	}
	if parsed.Season != 5 || parsed.Episode != 3 {
		t.Fatalf("season/episode=%d/%d want 5/3", parsed.Season, parsed.Episode)
	}
}
