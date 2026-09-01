package webui

import (
	"bytes"
	"testing"
)

func TestGamesG4PlayerOwnsKeyboardAndRetroPadInput(t *testing.T) {
	player, err := Static.ReadFile("static/games-player.js")
	if err != nil {
		t.Fatalf("read games-player.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`STORMFLIX GAME PLAYER G4`),
		[]byte(`respondToGlobalEvents:false`),
		[]byte(`input_auto_game_focus:true`),
		[]byte(`input_player1_a:'x'`),
		[]byte(`input_player1_b:'z'`),
		[]byte(`input_player1_select:'rshift'`),
		[]byte(`window.addEventListener('keydown',e=>handleKeyboard(e,true),{capture:true`),
		[]byte(`event.preventDefault();event.stopImmediatePropagation()`),
		[]byte(`instance.pressDown({button,player:1})`),
		[]byte(`instance.pressUp({button,player:1})`),
		[]byte(`keyboardPressed=new Map()`),
		[]byte(`releaseAllInputs()`),
		[]byte(`input_player1_a_btn:'1'`),
		[]byte(`input_player1_b_btn:'0'`),
		[]byte(`input_player1_select_btn:'8'`),
		[]byte(`input_player1_start_btn:'9'`),
		[]byte(`input_player1_up_btn:'12'`),
		[]byte(`input_player1_right_btn:'15'`),
	} {
		if !bytes.Contains(player, required) {
			t.Fatalf("Games G4 player missing input contract %q", required)
		}
	}
}

func TestGamesG42ViewportKeepsStableBackingSurfaceAndCSSFitsStage(t *testing.T) {
	player, err := Static.ReadFile("static/games-player.js")
	if err != nil {
		t.Fatalf("read games-player.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`ResizeObserver`),
		[]byte(`stage.getBoundingClientRect()`),
		[]byte(`size:emulatorSurfaceSize(current.platform)`),
		[]byte(`platformAspect(current.platform)`),
		[]byte(`canvas.style.setProperty('width'`),
		[]byte(`canvas.style.setProperty('height'`),
		[]byte(`video_force_aspect:true`),
		[]byte(`video_scale_integer:!!p.video.integerScale`),
	} {
		if !bytes.Contains(player, required) {
			t.Fatalf("Games G4.2 stable viewport missing %q", required)
		}
	}
	for _, forbidden := range [][]byte{
		[]byte(`instance.resize({width,height})`),
		[]byte(`video_force_aspect:p.video.display!=='stretch'`),
	} {
		if bytes.Contains(player, forbidden) {
			t.Fatalf("Games G4.2 must not mutate the RetroArch backing viewport after launch: %q", forbidden)
		}
	}
}

func TestGamesG42ShadersSwitchInsideRunningRetroArch(t *testing.T) {
	video, err := Static.ReadFile("static/games-g4-video.js")
	if err != nil {
		t.Fatalf("read games-g4-video.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`_cmd_set_shader`),
		[]byte(`stringToNewUTF8`),
		[]byte(`FS.writeFile`),
		[]byte(`/stormflix-shaders/`),
		[]byte(`applyLive(id)`),
		[]byte(`stormflix:game-filter-applied`),
		[]byte(`stormflix:game-started`),
		[]byte(`aplicado em tempo real`),
	} {
		if !bytes.Contains(video, required) {
			t.Fatalf("Games G4.2 live shader pipeline missing %q", required)
		}
	}
	for _, forbidden := range [][]byte{
		[]byte(`options.shader=id`),
		[]byte(`options.resolveShader=()=>buildBundle(id)`),
	} {
		if bytes.Contains(video, forbidden) {
			t.Fatalf("Games G4.2 should not require launch-time shader injection %q", forbidden)
		}
	}
}

func TestGamesG4SettingsAndTouchUseSinglePlayerAPI(t *testing.T) {
	g4, err := Static.ReadFile("static/games-g4.js")
	if err != nil {
		t.Fatalf("read games-g4.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`['quick','Jogo']`),
		[]byte(`['controls','Controles']`),
		[]byte(`['video','Vídeo']`),
		[]byte(`['emulator','Emulador']`),
		[]byte(`captureKeyboard`),
		[]byte(`captureGamepad`),
		[]byte(`navigator.getGamepads`),
		[]byte(`gamepadKeys`),
		[]byte(`standardButtons`),
		[]byte(`elementFromPoint`),
		[]byte(`releasePointer`),
		[]byte(`player()?.pressDown?.(input)`),
		[]byte(`player()?.pressUp?.(input)`),
		[]byte(`data-g4-touch-input`),
		[]byte(`data-video-smooth`),
		[]byte(`data-video-integer`),
		[]byte(`data-emu-rewind`),
		[]byte(`data-g4-apply`),
	} {
		if !bytes.Contains(g4, required) {
			t.Fatalf("Games G4 shell missing %q", required)
		}
	}
}

func TestGamesG4CSSKeepsCanvasContainedAndTouchOverlayNonCropping(t *testing.T) {
	css, err := Static.ReadFile("static/games-g4.css")
	if err != nil {
		t.Fatalf("read games-g4.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`object-fit:contain!important`),
		[]byte(`width:100%!important`),
		[]byte(`height:100%!important`),
		[]byte(`overflow:hidden!important`),
		[]byte(`.sf-g4-touch{position:absolute;inset:0`),
		[]byte(`pointer-events:none`),
		[]byte(`button.pressed`),
		[]byte(`orientation:landscape`),
	} {
		if !bytes.Contains(css, required) {
			t.Fatalf("Games G4 CSS missing %q", required)
		}
	}

	g42, err := Static.ReadFile("static/games-g42.css")
	if err != nil {
		t.Fatalf("read games-g42.css: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`align-items:center!important`),
		[]byte(`justify-content:center!important`),
		[]byte(`width:auto!important`),
		[]byte(`height:auto!important`),
	} {
		if !bytes.Contains(g42, required) {
			t.Fatalf("Games G4.2 CSS override missing %q", required)
		}
	}
}

func TestGamesG4ReplacesLegacyG3RuntimeInIndex(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`/games-player.js?v=g4`),
		[]byte(`/games-g4-session.js?v=g4`),
		[]byte(`/games-g4.js?v=g4`),
		[]byte(`/games-g4.css?v=g4`),
		[]byte(`/games-g42.css?v=g42`),
	} {
		if !bytes.Contains(index, required) {
			t.Fatalf("Games G4 index missing %q", required)
		}
	}
	if bytes.Contains(index, []byte(`/games-g3.js`)) {
		t.Fatal("legacy games-g3.js must not run alongside the G4 input owner")
	}
}
