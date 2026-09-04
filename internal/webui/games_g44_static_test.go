package webui

import (
	"bytes"
	"testing"
)

func TestGamesG44AssetsAreLoaded(t *testing.T) {
	manifest, err := Static.ReadFile("static/bundles/manifest.json")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`"games"`),
		[]byte(`/bundles/games.`),
	} {
		if !bytes.Contains(manifest, required) {
			t.Fatalf("Games G4.4 manifest missing %q", required)
		}
	}
}

func TestGamesG44RestoresCircularTouchAndExitChoice(t *testing.T) {
	js, err := Static.ReadFile("static/games-g44.js")
	if err != nil {
		t.Fatalf("read games-g44.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`data-g44-stick`),
		[]byte(`STICK_DEADZONE=.20`),
		[]byte(`requestAnimationFrame(flushStick)`),
		[]byte(`data-g44-save-exit`),
		[]byte(`data-g44-nosave-exit`),
		[]byte(`Sair sem salvar`),
		[]byte(`Cancelar`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.4 touch/exit flow missing %q", required)
		}
	}
}

func TestGamesG44HomeRailsAndMedia(t *testing.T) {
	js, err := Static.ReadFile("static/games-g44.js")
	if err != nil {
		t.Fatalf("read games-g44.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`Continuar jogando`),
		[]byte(`Jogos adicionados recentemente`),
		[]byte(`/api/v1/games/home`),
		[]byte(`game.screenshots||[]`),
		[]byte(`game.trailer_url`),
		[]byte(`gx-g44-shot-track`),
		[]byte(`const external=mutations.some`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Games G4.4 home/media missing %q", required)
		}
	}
}

func TestGamesG44ResponsiveAlignment(t *testing.T) {
	css, err := Static.ReadFile("static/games-g44.css")
	if err != nil {
		t.Fatalf("read games-g44.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`width:calc(100% - 9vw)!important`),
		[]byte(`.sf-g44-stick`),
		[]byte(`orientation:portrait`),
		[]byte(`max-height:43dvh!important`),
		[]byte(`backdrop-filter:none`),
		[]byte(`.gx-g44-shot-track`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games G4.4 CSS missing %q", required)
		}
	}
}
