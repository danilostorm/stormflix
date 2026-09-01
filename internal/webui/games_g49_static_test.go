package webui

import (
	"bytes"
	"testing"
)

func TestGamesG49SmartCollectionsLoaded(t *testing.T) {
	loader, err := Static.ReadFile("static/games-g48-home.js")
	if err != nil {
		t.Fatalf("read games-g48-home.js: %v", err)
	}
	style, err := Static.ReadFile("static/games-g48-home.css")
	if err != nil {
		t.Fatalf("read games-g48-home.css: %v", err)
	}
	if !bytes.Contains(loader, []byte(`/games-g49-collections.js?v=g49`)) {
		t.Fatal("Games G4.9 collections script is not loaded")
	}
	if !bytes.Contains(style, []byte(`/games-g49-collections.css?v=g49`)) {
		t.Fatal("Games G4.9 collections stylesheet is not loaded")
	}
}

func TestGamesG49CollectionsAreCrossPlatformTitleFamilies(t *testing.T) {
	js, err := Static.ReadFile("static/games-g49-collections.js")
	if err != nil {
		t.Fatalf("read games-g49-collections.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`buildCollections(games)`),
		[]byte(`commonPrefix`),
		[]byte(`sequelBase`),
		[]byte(`SÉRIES E FRANQUIAS`),
		[]byte(`Coleções automáticas`),
		[]byte(`Explorar por plataforma`),
		[]byte(`data-g49-collection`),
		[]byte(`data-game-open`),
		[]byte(`group.platforms`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.9 collections contract missing %q", required)
		}
	}
}

func TestGamesG49CollectionsResponsiveGrid(t *testing.T) {
	css, err := Static.ReadFile("static/games-g49-collections.css")
	if err != nil {
		t.Fatalf("read games-g49-collections.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`.g49-collection-grid`),
		[]byte(`repeat(auto-fill,minmax(280px,1fr))`),
		[]byte(`@media(min-width:1700px)`),
		[]byte(`@media(max-width:720px)`),
		[]byte(`@media(max-width:480px)`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games G4.9 responsive collection contract missing %q", required)
		}
	}
}
