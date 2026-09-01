package webui

import (
	"bytes"
	"testing"
)

func TestGamesG45KeepsNativeMediaCatalogAndAddsGames(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !bytes.Contains(index, []byte(`/games-g45-home-compat.js?v=g45`)) {
		t.Fatal("Games compatibility guard is not loaded")
	}

	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`.content-row:not([data-g44-home-row])`),
		[]byte(`window.loadHome`),
		[]byte(`window.allFeedItems`),
		[]byte(`window.renderRows`),
		[]byte(`media_type==='movie'`),
		[]byte(`media_type==='series'`),
		[]byte(`media_type==='anime'`),
		[]byte(`removeHomeGameRowsOutsideHome`),
		[]byte(`Games rails are the only surviving rows`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games native catalog guard missing %q", required)
		}
	}
}

func TestGamesG46PlacesRailsNearTopWithoutTakingOverHome(t *testing.T) {
	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`data-g44-home-row=\"continue\"`),
		[]byte(`data-g44-home-row=\"recent\"`),
		[]byte(`nativeRow('Em alta agora')`),
		[]byte(`nativeRow('Em alta nesta semana')`),
		[]byte(`nativeRow('Lançamentos')`),
		[]byte(`placeHomeGameRows`),
		[]byte(`insertAdjacentElement('afterend',continued)`),
		[]byte(`insertAdjacentElement('afterend',recent)`),
		[]byte(`avoiding two consecutive game rails`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.6 Home placement contract missing %q", required)
		}
	}
}
