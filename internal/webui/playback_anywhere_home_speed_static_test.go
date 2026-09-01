package webui

import (
	"bytes"
	"testing"
)

func TestPlaybackAnywhereDetailActionIsFirstClass(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`id="detail-anywhere"`),
		[]byte(`Reproduzir em`),
		[]byte(`/playback-anywhere.js?v=2`),
		[]byte(`/source-selector.js?v=3`),
		[]byte(`/catalog-performance.js?v=2`),
	} {
		if !bytes.Contains(index, required) {
			t.Fatalf("detail Playback Anywhere missing %q", required)
		}
	}

	js, err := Static.ReadFile("static/playback-anywhere.js")
	if err != nil {
		t.Fatalf("read playback-anywhere.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`openForMedia(media)`),
		[]byte(`document.body.appendChild(panel)`),
		[]byte(`targetMedia`),
		[]byte(`closest?.('#detail-anywhere')`),
		[]byte(`selected._logical_id||selected.id`),
		[]byte(`window.StormFlixPlaybackAnywhere`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("detail Playback Anywhere implementation missing %q", required)
		}
	}
}

func TestSourceSelectorNoLongerShowsRedundantServerCopy(t *testing.T) {
	js, err := Static.ReadFile("static/source-selector.js")
	if err != nil {
		t.Fatalf("read source-selector.js: %v", err)
	}
	if bytes.Contains(js, []byte(`Escolha o servidor. O título e os metadados são únicos.`)) {
		t.Fatal("redundant source-selector helper copy must stay removed")
	}
	for _, required := range [][]byte{
		[]byte(`window.sfSelectedDetailMedia`),
		[]byte(`_logical_id:Number(detail.id)`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("selected physical source integration missing %q", required)
		}
	}
}

func TestGamesCannotPaintBeforeNativeHome(t *testing.T) {
	compat, err := Static.ReadFile("static/games-g45-home-compat.js")
	if err != nil {
		t.Fatalf("read games-g45-home-compat.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`sf-native-home-ready`),
		[]byte(`body:not(.sf-native-home-ready) #rows > [data-g44-home-row]{display:none!important}`),
		[]byte(`nativeRows().length>=2`),
		[]byte(`stormflix:native-home-ready`),
	} {
		if !bytes.Contains(compat, required) {
			t.Fatalf("native Home paint gate missing %q", required)
		}
	}

	cache, err := Static.ReadFile("static/games-instant-cache.js")
	if err != nil {
		t.Fatalf("read games-instant-cache.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`'/games/home'`),
		[]byte(`'/games?limit=500'`),
		[]byte(`nativeRowsReady()`),
		[]byte(`window.request=cachedRequest`),
	} {
		if !bytes.Contains(cache, required) {
			t.Fatalf("Games warm cache missing %q", required)
		}
	}
}
