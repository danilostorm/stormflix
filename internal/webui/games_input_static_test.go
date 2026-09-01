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
		[]byte(`input_player1_select:'rshift'`),
		[]byte(`INPUT_ASSET_VERSION='g34'`),
		[]byte(`/games-g3-inputs.js?v=${INPUT_ASSET_VERSION}`),
		[]byte(`/games-g3-inputs.css?v=${INPUT_ASSET_VERSION}`),
		[]byte(`instance.pressDown({button,player:1})`),
		[]byte(`instance.pressUp({button,player:1})`),
		[]byte(`pressedInputs=new Set()`),
		[]byte(`releaseAllInputs()`),
		[]byte(`instance.resize({width,height})`),
		[]byte(`aspectByPlatform`),
		[]byte(`stormflix:game-started`),
	} {
		if !bytes.Contains(player, required) {
			t.Fatalf("Games player missing direct input/resize contract %q", required)
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
		[]byte(`data-sf-stick`),
		[]byte(`STICK_DEADZONE=.20`),
		[]byte(`Math.atan2(dy,dx)`),
		[]byte(`['down','right']`),
		[]byte(`['up','left']`),
		[]byte(`button.hasPointerCapture?.(event.pointerId)`),
		[]byte(`button.releasePointerCapture(event.pointerId)`),
		[]byte(`event.buttons===0`),
		[]byte(`resetVirtualInputs()`),
		[]byte(`removeLegacyController(overlay)`),
		[]byte(`type="button"`),
		[]byte(`player()?.resize?.()`),
	} {
		if !bytes.Contains(input, required) {
			t.Fatalf("Games virtual pad missing control/layout %q", required)
		}
	}
	for _, forbidden := range [][]byte{
		[]byte(`class="sf-pad-dpad"`),
		[]byte(`class="diag ul"`),
		[]byte(`buttonFromPoint(`),
		[]byte(`button.setPointerCapture?.(pointerId)`),
	} {
		if bytes.Contains(input, forbidden) {
			t.Fatalf("Games virtual pad contains obsolete/cross-input behavior %q", forbidden)
		}
	}

	css, err := Static.ReadFile("static/games-g3-inputs.css")
	if err != nil {
		t.Fatalf("read games-g3-inputs.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`calc((100vh - 176px) * 4 / 3)`),
		[]byte(`.sf-pad-stick{`),
		[]byte(`.sf-pad-stick-thumb{`),
		[]byte(`border-radius:50%`),
		[]byte(`.sf-pad-face.diamond`),
		[]byte(`.sf-pad-shoulders`),
		[]byte(`orientation:landscape`),
		[]byte(`button.pressed{`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games responsive input CSS missing %q", required)
		}
	}
	for _, forbidden := range [][]byte{
		[]byte(`.sf-pad-dpad`),
		[]byte(`.sf-pad-face.diamond .a{grid-column:3;grid-row:2;background:`),
	} {
		if bytes.Contains(css, forbidden) {
			t.Fatalf("Games responsive input CSS contains stale/false pressed styling %q", forbidden)
		}
	}
}
