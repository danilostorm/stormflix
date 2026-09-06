package webui

import (
	"bytes"
	"testing"
)

func TestDeviceAware4KCatalogAndPlaybackWiring(t *testing.T) {
	index, err := Static.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(index, []byte(`/device-capabilities.js`)) {
		t.Fatal("device capability probe must load before the catalog")
	}
	device, err := Static.ReadFile("static/device-capabilities.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range [][]byte{
		[]byte(`mediaCapabilities?.decodingInfo`),
		[]byte(`result?.supported&&result?.smooth`),
		[]byte(`client_max_height`),
		[]byte(`sfCatalogCapabilityQuery`),
	} {
		if !bytes.Contains(device, required) {
			t.Fatalf("device-aware catalog probe missing %q", required)
		}
	}
	core, err := Static.ReadFile("static/playback-core-v53.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range [][]byte{[]byte(`video_profiles:videoProfiles`), []byte(`StormFlixShell?.playbackRequest`)} {
		if !bytes.Contains(core, required) {
			t.Fatalf("device-aware PlaybackPlan wiring missing %q", required)
		}
	}
}
