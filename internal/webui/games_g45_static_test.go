package webui

import (
	"bytes"
	"testing"
)

func TestGamesG47KeepsNativeHomeAuthoritative(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !bytes.Contains(index, []byte(`/games-g45-home-compat.js?v=g47`)) {
		t.Fatal("Games G4.7 Home watchdog is not cache-busted/loaded")
	}

	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`Games is NEVER allowed to be the only owner`),
		[]byte(`gameRows().forEach(node=>node.remove())`),
		[]byte(`window.showHome`),
		[]byte(`window.loadHome`),
		[]byte(`if(gameCount>0&&nativeCount===0){scheduleRepair(0);return}`),
		[]byte(`if(games.length&&natives.length===0){scheduleRepair(0);return}`),
		[]byte(`removeGameRowsOutsideHome`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.7 native Home invariant missing %q", required)
		}
	}
}

func TestGamesG47PlacesRailsOnlyAfterNativeRowsExist(t *testing.T) {
	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`[data-g44-home-row=\"continue\"]`),
		[]byte(`[data-g44-home-row=\"recent\"]`),
		[]byte(`nativeRow('Em alta agora')`),
		[]byte(`nativeRow('Em alta nesta semana')`),
		[]byte(`nativeRow('Lançamentos')`),
		[]byte(`if(natives.length<2||!games.length)return`),
		[]byte(`insertAdjacentElement('afterend',continued)`),
		[]byte(`insertAdjacentElement('afterend',recent)`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.7 safe placement contract missing %q", required)
		}
	}
}
