package webui

import (
	"bytes"
	"testing"
)

func TestGamesPlayerUsesDirectRetroPadInputs(t *testing.T) {
	player, err := Static.ReadFile("static/games-player.js")
	if err != nil {
		t.Fatalf("read games-player.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`input_player1_a:'x'`),
		[]byte(`input_player1_b:'z'`),
		[]byte(`input_player1_x:'s'`),
		[]byte(`input_player1_y:'a'`),
		[]byte(`pressDown,pressUp`),
		[]byte(`stormflix:game-started`),
		[]byte(`/games-g3-inputs.js`),
	} {
		if !bytes.Contains(player, required) {
			t.Fatalf("Games player missing direct input contract %q", required)
		}
	}
	if bytes.Contains(player, []byte(`overlay.addEventListener('keydown',captureGameKeys,true)`)) {
		t.Fatal("Games player must not intercept gameplay keyboard events before RetroArch")
	}
}

func TestGamesVirtualPadCoversPlatformControls(t *testing.T) {
	input, err := Static.ReadFile("static/games-g3-inputs.js")
	if err != nil {
		t.Fatalf("read games-g3-inputs.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`snes:{name:'Super Nintendo',shoulders:[['l','L'],['r','R']],face:[['y','Y'],['x','X'],['b','B'],['a','A']]}`),
		[]byte(`gba:{name:'Game Boy Advance',shoulders:[['l','L'],['r','R']]`),
		[]byte(`genesis:{name:'Mega Drive / Genesis'`),
		[]byte(`api.pressDown?.(input)`),
		[]byte(`api.pressUp?.(input)`),
		[]byte(`data-inputs="up,left"`),
	} {
		if !bytes.Contains(input, required) {
			t.Fatalf("Games virtual pad missing control/layout %q", required)
		}
	}

	css, err := Static.ReadFile("static/games-g3-inputs.css")
	if err != nil {
		t.Fatalf("read games-g3-inputs.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`calc((100vh - 176px) * 4 / 3)`),
		[]byte(`.sf-pad-face.diamond`),
		[]byte(`.sf-pad-shoulders`),
		[]byte(`orientation:landscape`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games responsive input CSS missing %q", required)
		}
	}
}
