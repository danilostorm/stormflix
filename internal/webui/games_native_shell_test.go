package webui

import (
	"bytes"
	"testing"
)

func TestGamesUsesNativeStormFlixShell(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !bytes.Contains(index, []byte(`/games-native-shell.css`)) {
		t.Fatal("Games native-shell stylesheet is not loaded by the StormFlix app")
	}

	css, err := Static.ReadFile("static/games-native-shell.css")
	if err != nil {
		t.Fatalf("read Games native shell stylesheet: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`.games-view{position:relative!important`),
		[]byte(`.gx-brand,.gx-user{display:none!important}`),
		[]byte(`body.games-mode #topbar{z-index:900;`),
		[]byte(`grid-template-columns:repeat(6,minmax(0,1fr))!important`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games native shell is missing required rule %q", required)
		}
	}
}
