package metadata

import "testing"

func TestScannerIdentityUsesFirstFolderBelowLibraryRoot(t *testing.T) {
	got := deriveScannerIdentity(
		"/media/akumanimes/Desenhos/Pica-Pau e seus Amigos/Remux/002PP-BD1080pRemux.mkv",
		"/media/akumanimes/Desenhos",
		"animation_series",
	)
	if got.SeriesTitle != "Pica-Pau e seus Amigos" {
		t.Fatalf("SeriesTitle=%q want %q", got.SeriesTitle, "Pica-Pau e seus Amigos")
	}
	if got.Season != 1 || got.Episode != 2 {
		t.Fatalf("season/episode=%d/%d want 1/2", got.Season, got.Episode)
	}
	if got.SeriesKey != normalizeTitle("Pica-Pau e seus Amigos") {
		t.Fatalf("SeriesKey=%q", got.SeriesKey)
	}
}

func TestScannerIdentitySupportsSourcePointingAtShowFolder(t *testing.T) {
	got := deriveScannerIdentity(
		"/media/Desenhos/Pica-Pau e seus Amigos/Remux/003PP-BD1080pRemux.mkv",
		"/media/Desenhos/Pica-Pau e seus Amigos",
		"animation_series",
	)
	if got.SeriesTitle != "Pica-Pau e seus Amigos" {
		t.Fatalf("SeriesTitle=%q want %q", got.SeriesTitle, "Pica-Pau e seus Amigos")
	}
	if got.Season != 1 || got.Episode != 3 {
		t.Fatalf("season/episode=%d/%d want 1/3", got.Season, got.Episode)
	}
}

func TestScannerIdentityReadsLooseBrazilianSeasonFolder(t *testing.T) {
	got := deriveScannerIdentity(
		"/media/Desenhos/Batman - A Série Animada/5ª Temporada - Stormbrasil/07 - Episodio.mkv",
		"/media/Desenhos",
		"animation_series",
	)
	if got.SeriesTitle != "Batman - A Série Animada" {
		t.Fatalf("SeriesTitle=%q want Batman - A Série Animada", got.SeriesTitle)
	}
	if got.Season != 5 || got.Episode != 7 {
		t.Fatalf("season/episode=%d/%d want 5/7", got.Season, got.Episode)
	}
}

func TestScannerIdentityDoesNotPromoteTechnicalFolder(t *testing.T) {
	got := deriveScannerIdentity(
		"/media/Desenhos/Looney Tunes/BluRay/1080p/014LT-BD1080pRemux.mkv",
		"/media/Desenhos",
		"animation_series",
	)
	if got.SeriesTitle != "Looney Tunes" {
		t.Fatalf("SeriesTitle=%q want Looney Tunes", got.SeriesTitle)
	}
}
