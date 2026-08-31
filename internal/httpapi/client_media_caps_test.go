package httpapi

import "testing"

func TestClientAllows4KMediaRequiresResolutionAndCodec(t *testing.T) {
	tech := technicalSnapshot{Status: "ok", Height: 2160, VideoCodec: "hevc"}
	caps := clientMediaCaps{Explicit: true, MaxHeight: 1080, VideoCodecs: map[string]bool{"hevc": true}, HDRTypes: map[string]bool{}}
	if clientAllows4KMedia(caps, tech) {
		t.Fatal("1080p client must not receive UHD shelf item")
	}
	caps.MaxHeight = 2160
	caps.VideoCodecs = map[string]bool{"h264": true}
	if clientAllows4KMedia(caps, tech) {
		t.Fatal("client without HEVC must not receive HEVC UHD shelf item")
	}
	caps.VideoCodecs["hevc"] = true
	if !clientAllows4KMedia(caps, tech) {
		t.Fatal("4K HEVC-capable client should receive UHD shelf item")
	}
}

func TestClientAllows4KMediaPreservesLegacyAnd1080p(t *testing.T) {
	legacy := clientMediaCaps{}
	if !clientAllows4KMedia(legacy, technicalSnapshot{Status: "ok", Height: 2160, VideoCodec: "hevc"}) {
		t.Fatal("legacy clients without explicit hints must preserve old behavior")
	}
	caps := clientMediaCaps{Explicit: true, MaxHeight: 720, VideoCodecs: map[string]bool{"h264": true}, HDRTypes: map[string]bool{}}
	if !clientAllows4KMedia(caps, technicalSnapshot{Status: "ok", Height: 1080, VideoCodec: "h264"}) {
		t.Fatal("4K gate must not hide ordinary HD/FHD catalog items")
	}
}

func TestClientAllows4KMediaHonorsKnownHDR(t *testing.T) {
	tech := technicalSnapshot{Status: "ok", Height: 2160, VideoCodec: "hevc", HDR: "hdr10"}
	caps := clientMediaCaps{Explicit: true, MaxHeight: 2160, VideoCodecs: map[string]bool{"hevc": true}, HDRKnown: true, HDRTypes: map[string]bool{}}
	if clientAllows4KMedia(caps, tech) {
		t.Fatal("known SDR-only client must not receive HDR UHD item")
	}
	caps.HDRTypes["hdr10"] = true
	if !clientAllows4KMedia(caps, tech) {
		t.Fatal("HDR10-capable 4K client should receive HDR10 UHD item")
	}
}

func TestDeviceGateAppliesOnlyToDedicatedUHDSmartShelf(t *testing.T) {
	if shouldGateUHDCategoryByDevice("rules", categoryRules{MinHeight: 1080}) {
		t.Fatal("ordinary FHD or genre smart rails must not hide UHD titles")
	}
	if !shouldGateUHDCategoryByDevice("rules", categoryRules{MinHeight: 2000}) {
		t.Fatal("dedicated UHD smart shelf should use device capability gate")
	}
	if shouldGateUHDCategoryByDevice("libraries", categoryRules{MinHeight: 2160}) {
		t.Fatal("library-only category does not apply technical rules and must not become a device access gate")
	}
}
