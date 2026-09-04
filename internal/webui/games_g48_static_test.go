package webui

import (
	"bytes"
	"testing"
)

func TestGamesG48HomeDashboardLoaded(t *testing.T) {
	manifest, err := Static.ReadFile("static/bundles/manifest.json")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`"games"`),
		[]byte(`/bundles/games.`),
	} {
		if !bytes.Contains(manifest, required) {
			t.Fatalf("Games G4.8 asset missing %q", required)
		}
	}
}

func TestGamesG48PersonalizesAndDeduplicatesHome(t *testing.T) {
	js, err := Static.ReadFile("static/games-g48-home.js")
	if err != nil {
		t.Fatalf("read games-g48-home.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`RETOMAR ÚLTIMA PARTIDA`),
		[]byte(`Continue seus outros jogos`),
		[]byte(`card.remove()`),
		[]byte(`const seen=new Set()`),
		[]byte(`removeDuplicateCards(favorites,seen)`),
		[]byte(`removeDuplicateCards(recent,seen)`),
		[]byte(`removeDuplicateCards(ready,seen)`),
		[]byte(`Explore sua biblioteca`),
		[]byte(`while(grid.children.length<4`),
		[]byte(`Biblioteca`),
		[]byte(`Saves`),
		[]byte(`Coleções`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.8 Home contract missing %q", required)
		}
	}
}

func TestGamesG48DashboardIsResponsive(t *testing.T) {
	css, err := Static.ReadFile("static/games-g48-home.css")
	if err != nil {
		t.Fatalf("read games-g48-home.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`.g48-dashboard`),
		[]byte(`.g48-activity-grid`),
		[]byte(`@media(min-width:1700px)`),
		[]byte(`@media(max-width:1180px)`),
		[]byte(`@media(max-width:760px)`),
		[]byte(`@media(max-width:440px)`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games G4.8 responsive contract missing %q", required)
		}
	}
}
