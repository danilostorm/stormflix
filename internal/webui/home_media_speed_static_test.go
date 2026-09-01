package webui

import (
	"bytes"
	"testing"
)

func TestVideoHomeUsesPerProfileSnapshotBeforeRefresh(t *testing.T) {
	js, err := Static.ReadFile("static/catalog-performance.js")
	if err != nil {
		t.Fatalf("read catalog-performance.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`stormflix.home.snapshot.v2:`),
		[]byte(`sessionStorage`),
		[]byte(`window.sfProfiles?.current?.()`),
		[]byte(`if(cached)paintSnapshot(cached)`),
		[]byte(`return await baseLoadHome()`),
		[]byte(`fetchpriority="high"`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("instant video Home contract missing %q", required)
		}
	}
}

func TestPlaybackAnywhereIsAnchoredToDetailButton(t *testing.T) {
	js, err := Static.ReadFile("static/source-selector.js")
	if err != nil {
		t.Fatalf("read source-selector.js: %v", err)
	}
	for _, required := range [][]byte{
		[]byte(`#detail-anywhere,#sf-anywhere-toggle`),
		[]byte(`getBoundingClientRect()`),
		[]byte(`panel.style.left`),
		[]byte(`panel.style.right='auto'`),
		[]byte(`window.innerWidth<=700`),
	} {
		if !bytes.Contains(js, required) {
			t.Fatalf("Playback Anywhere anchor contract missing %q", required)
		}
	}
}
