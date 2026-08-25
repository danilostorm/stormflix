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

func TestParseLegacyRoboCopRelease(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Antigos/Robocop.87.DC.TrialBD1080p.MemoriadaTV.Medio.mkv`, "movies")
	if p.Title != "Robocop" {
		t.Fatalf("expected clean title, got %q", p.Title)
	}
	if p.Year != 1987 {
		t.Fatalf("expected 1987, got %d", p.Year)
	}
}

func TestParseLegacyHighlanderSequenceAndYear(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Antigos/Highlander3.94.BD1080p.MemoriadaTV.Remux.mkv`, "movies")
	if p.Title != "Highlander 3" {
		t.Fatalf("expected Highlander 3, got %q", p.Title)
	}
	if p.Year != 1994 {
		t.Fatalf("expected 1994, got %d", p.Year)
	}
}

func TestParseLegacyHighlanderRenegadeEdition(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Antigos/Highlander2.91.Renegade.BD1080p.MemoriadaTV.Remux.V2.mkv`, "movies")
	if p.Title != "Highlander 2" {
		t.Fatalf("expected Highlander 2, got %q", p.Title)
	}
	if p.Year != 1991 {
		t.Fatalf("expected 1991, got %d", p.Year)
	}
}

func TestParseMatrixReleaseWithParenthesizedYear(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Antigos/Matrix Revolutions (2003) (1080p BluRay x265 10bit-Tigole) DUAL-TaiBala.mkv`, "movies")
	if p.Title != "Matrix Revolutions" {
		t.Fatalf("expected Matrix Revolutions, got %q", p.Title)
	}
	if p.Year != 2003 {
		t.Fatalf("expected 2003, got %d", p.Year)
	}
}

func TestParseOld007TwoDigitYearAndGluedNao(t *testing.T) {
	p := ParseFilename(`/media/Guilherme/Filmes Antigos/007AmanhaNAOmorre.97.TetraBD1080p.MemoriadaTV.Remux.mkv`, "movies")
	if p.Year != 1997 {
		t.Fatalf("expected 1997, got %d", p.Year)
	}
	if strings.Contains(p.Title, "NAO") || !strings.Contains(strings.ToLower(p.Title), "nao") {
		t.Fatalf("expected glued NAO to be normalized, got %q", p.Title)
	}
}
