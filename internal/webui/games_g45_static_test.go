package webui

import (
	"bytes"
	"testing"
)

func TestGamesG410KeepsNativeHomeAuthoritative(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !bytes.Contains(index, []byte(`/games-g45-home-compat.js?v=g410`)) {
		t.Fatal("Games G4.10 Home paint gate is not cache-busted/loaded")
	}

	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`body:not(.sf-native-home-ready) #rows > [data-g44-home-row]{display:none!important}`),
		[]byte(`gameRows().forEach(node=>node.remove())`),
		[]byte(`window.showHome`),
		[]byte(`window.loadHome`),
		[]byte(`if(gameCount>0&&nativeCount<2){scheduleRepair(0);return}`),
		[]byte(`if(natives.length<2){syncPaintGate();if(games.length)scheduleRepair(0);return}`),
		[]byte(`stormflix:native-home-ready`),
		[]byte(`removeGameRowsOutsideHome`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.10 native Home invariant missing %q", required)
		}
	}
}

func TestGamesG410PlacesRailsOnlyAfterNativeRowsExist(t *testing.T) {
	js, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`[data-g44-home-row="continue"]`),
		[]byte(`[data-g44-home-row="recent"]`),
		[]byte(`nativeRow('Em alta agora')`),
		[]byte(`nativeRow('Em alta nesta semana')`),
		[]byte(`nativeRow('Lançamentos')`),
		[]byte(`nativeRows().length>=2`),
		[]byte(`insertAdjacentElement('afterend',continued)`),
		[]byte(`insertAdjacentElement('afterend',recent)`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.10 safe placement contract missing %q", required)
		}
	}
}
