package metadata

import (
	"strings"
	"testing"
)

func TestParseAnimeMovieUsesFranchiseFolder(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Animes/Dragon Ball Z/Filme 15 - O Renascimento de Freeza/Filme 15 - O Renascimento de Freeza.mkv`, "anime")
	if p.Title != "Dragon Ball Z O Renascimento de Freeza" {
		t.Fatalf("unexpected title: %q", p.Title)
	}
	if !p.LikelyMovie {
		t.Fatal("expected anime movie classification")
	}
	joined := strings.Join(p.SearchTitles(), " | ")
	if !strings.Contains(joined, "O Renascimento de Freeza") {
		t.Fatalf("expected subtitle alternate, got %s", joined)
	}
}

func TestParseGenericVideoplaybackUsesParentFolder(t *testing.T) {
	p := ParseFilename(`/media/Akumanimes/Filmes Animes/afro samurai resurrection/videoplayback.mp4`, "anime")
	if p.Title != "afro samurai resurrection" {
		t.Fatalf("unexpected title: %q", p.Title)
	}
	if !p.LikelyMovie {
		t.Fatal("expected movie classification from Filmes Animes path")
	}
}

func TestParseRemovesVHSMarker(t *testing.T) {
	p := ParseFilename(`/media/Akumanimes/Filmes Animes/Spawn O Soldado do Inferno/Spawn O Soldado do Inferno(VHS).mp4`, "anime")
	if strings.Contains(strings.ToLower(p.Title), "vhs") {
		t.Fatalf("VHS marker was not removed: %q", p.Title)
	}
	if p.Title != "Spawn O Soldado do Inferno" {
		t.Fatalf("unexpected title: %q", p.Title)
	}
}

func TestParseRemovesAnimeMovieNumberInsideTitle(t *testing.T) {
	p := ParseFilename(`/media/Akumanimes/Filmes Animes/Os Cavaleiros do Zodíaco/Os Cavaleiros do Zodíaco Filme6 - A Lenda do Santuário.mkv`, "anime")
	if strings.Contains(strings.ToLower(p.Title), "filme6") || strings.Contains(strings.ToLower(p.Title), "filme 6") {
		t.Fatalf("movie index was not removed: %q", p.Title)
	}
	if !strings.Contains(p.Title, "Os Cavaleiros do Zodíaco") || !strings.Contains(p.Title, "A Lenda do Santuário") {
		t.Fatalf("important title parts were lost: %q", p.Title)
	}
}

func TestParseSimpleNumberedSeriesEpisodeUsesFolder(t *testing.T) {
	p := ParseFilename(`/media/Akumanimes/Animes/Dragon Quest - Dai no Daibouken/Fly (Dragon Quest - Dai no Daibouken) - 01.mkv`, "series")
	if p.Title != "Dragon Quest - Dai no Daibouken" {
		t.Fatalf("expected parent series title, got %q", p.Title)
	}
	if p.Season != 1 || p.Episode != 1 {
		t.Fatalf("expected S01E01, got S%02dE%02d", p.Season, p.Episode)
	}
	if p.LikelyMovie {
		t.Fatal("numbered series episode was classified as a movie")
	}
}

func TestMixedAnimeMovieKeepsMovieClassification(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Animes/Dragon Ball Z/Filme 15 - O Renascimento de Freeza.mkv`, "mixed")
	if !p.LikelyMovie {
		t.Fatal("mixed anime movie should still be classified as a movie")
	}
	if strings.Contains(strings.ToLower(p.Title), "filme 15") {
		t.Fatalf("movie sequence marker should be removed: %q", p.Title)
	}
}
