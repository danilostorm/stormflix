package webui

import (
	"bytes"
	"testing"
)

func TestGamesG43SaveChoiceLoadAndReliableClose(t *testing.T) {
	player, err := Static.ReadFile("static/games-player.js")
	if err != nil {
		t.Fatalf("read games-player.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`STORMFLIX GAME PLAYER G4.3`),
		[]byte(`Como você quer iniciar?`),
		[]byte(`data-game-continue`),
		[]byte(`data-game-new`),
		[]byte(`mode!=='new'&&current.saves?.sram?.exists`),
		[]byte(`async function loadSave()`),
		[]byte(`instance.loadState(state)`),
		[]byte(`data-game-load`),
		[]byte(`withTimeout(saveAll(true),5000`),
		[]byte(`lastGamepadSignature`),
		[]byte(`signature!==lastGamepadSignature`),
	} {
		if !bytes.Contains(player, required) {
			t.Fatalf("Games G4.3 player missing %q", required)
		}
	}
}

func TestGamesG43UsesFullPageDetailWithoutDownloadAction(t *testing.T) {
	ui, err := Static.ReadFile("static/games-g43-ui.js")
	if err != nil {
		t.Fatalf("read games-g43-ui.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`gx-game-page`),
		[]byte(`data-g43-play`),
		[]byte(`data-g43-tab="details"`),
		[]byte(`data-g43-tab="saves"`),
		[]byte(`data-g43-tab="file"`),
		[]byte(`data-g43-load-save`),
		[]byte(`document.addEventListener('click'`),
		[]byte(`stopImmediatePropagation()`),
	} {
		if !bytes.Contains(ui, required) {
			t.Fatalf("Games G4.3 detail UI missing %q", required)
		}
	}
	if bytes.Contains(ui, []byte(`data-g43-download`)) {
		t.Fatal("Games G4.3 must not expose a download action")
	}
}

func TestGamesG43ResponsiveShellAndCinemaChrome(t *testing.T) {
	css, err := Static.ReadFile("static/games-g43.css")
	if err != nil {
		t.Fatalf("read games-g43.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`--gx-shell-width`),
		[]byte(`.gx-g43-hero-body`),
		[]byte(`grid-template-columns:260px minmax(0,1fr)`),
		[]byte(`scrollbar-gutter:stable`),
		[]byte(`sf-g41-ui-hidden`),
		[]byte(`height:0!important`),
		[]byte(`align-items:center!important`),
		[]byte(`justify-content:center!important`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games G4.3 CSS missing %q", required)
		}
	}
}

func TestGamesG43AssetsAreLoaded(t *testing.T) {
	index, err := Static.ReadFile("static/feature-loader.js")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`manifest.json`),
		[]byte(`load('games')`),
	} {
		if !bytes.Contains(index, required) {
			t.Fatalf("Games G4.3 bundle loader missing %q", required)
		}
	}
}
