package webui

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestPlaybackDependenciesArePinnedAndSelfHosted(t *testing.T) {
	expected := map[string]string{
		"static/vendor-hls-1.7.1.min.js":               "6cfad701a61fb8a99add5e84449e64661169b0652bf44ceb2a28465c8817b5f1",
		"static/vendor-hevc-hls-plugin-0.1.2.js":       "0b2bdd6000f9d6d6cc37158d67e8cf6749d1d6220d512be0586a793c002d9d02",
		"static/vendor-hevc-transcode-worker-1.4.2.js": "ccacd072c7ca06deaa4711c52403a456ca0c6354ec7a9d3ac3d01864c9ed9125",
		"static/vendor-hevc-decode-1.4.2.js":           "1177cbd2c390bb1c5688b79c525a5847350858f6d31ed2a6e88044c34daae18f",
		"static/vendor-hevc-decode-1.4.2.wasm":         "b3099fcb5d6a74c536a48ffd67232ae45fc6b8cfec9bd52d6f8f734cd15e8a86",
	}
	for name, wanted := range expected {
		body, err := Static.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != wanted {
			t.Fatalf("%s checksum=%s, want %s", name, got, wanted)
		}
	}
	for _, name := range []string{"static/index.html", "static/playback-core-v53.js"} {
		body, err := Static.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, remote := range []string{"cdn.jsdelivr.net", "esm.sh", "unpkg.com"} {
			if strings.Contains(text, remote) {
				t.Fatalf("%s still depends on %s at runtime", name, remote)
			}
		}
	}
}
