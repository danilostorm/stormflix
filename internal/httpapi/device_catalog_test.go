package httpapi

import (
	"testing"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
	"github.com/danilostorm/stormflix/internal/transcode"
)

func TestSelectCatalogVersionSubstitutes1080pForIncompatible4K(t *testing.T) {
	caps := clientMediaCaps{Explicit: true, MaxHeight: 1080, VideoCodecs: map[string]bool{"h264": true}, HDRTypes: map[string]bool{}}
	versions := []media.Version{
		{ID: 10, Label: "4K", Height: 2160, VideoCodec: "hevc", TechnicalStatus: "ok"},
		{ID: 20, Label: "1080p", Height: 1080, VideoCodec: "h264", TechnicalStatus: "ok"},
	}
	selected, ok, _ := selectCatalogVersion(caps, 10, versions)
	if !ok || selected.ID != 20 {
		t.Fatalf("selected=%#v ok=%v, want media 20", selected, ok)
	}
}

func TestSelectCatalogVersionHidesKnown4KOnlyTitle(t *testing.T) {
	caps := clientMediaCaps{Explicit: true, MaxHeight: 1080, VideoCodecs: map[string]bool{"h264": true}, HDRTypes: map[string]bool{}}
	_, ok, _ := selectCatalogVersion(caps, 10, []media.Version{{ID: 10, Label: "4K", Height: 2160, VideoCodec: "hevc", TechnicalStatus: "ok"}})
	if ok {
		t.Fatal("known incompatible 4K-only title must be hidden")
	}
}

func TestCPU4KTranscodeIsBlockedButHardwareIsAllowed(t *testing.T) {
	base := playback.Plan{Available: true, Mode: playback.ModeVideoTranscode, VideoTranscode: true, VideoHeight: 2160, VideoCodec: "h264"}
	blocked := applyCPU4KTranscodeBlock(base, transcode.EngineStatus{VideoEncoders: []string{"libx264"}, PreferredH264: "libx264"})
	if blocked.Available || blocked.ReasonCode != "cpu_4k_transcode_blocked" {
		t.Fatalf("CPU 4K transcode was not blocked: %#v", blocked)
	}
	allowed := applyCPU4KTranscodeBlock(base, transcode.EngineStatus{VideoEncoders: []string{"h264_nvenc", "libx264"}, NVIDIADevice: "/dev/nvidia0", PreferredH264: "h264_nvenc"})
	if !allowed.Available || allowed.Encoder != "h264_nvenc" || allowed.HardwareAcceleration != "nvidia" {
		t.Fatalf("hardware 4K transcode should remain available: %#v", allowed)
	}
}

func TestCatalog4KCompatibilityCases(t *testing.T) {
	uhd := media.Version{ID: 10, Width: 3840, Height: 1600, VideoCodec: "hevc", TechnicalStatus: "ok"}
	fhd := media.Version{ID: 20, Width: 1920, Height: 1080, VideoCodec: "h264", TechnicalStatus: "ok"}
	for _, tc := range []struct {
		name     string
		caps     clientMediaCaps
		versions []media.Version
		want     int64
	}{
		{"cropped 4K hidden", clientMediaCaps{Explicit: true, MaxHeight: 1080}, []media.Version{uhd}, 0},
		{"cropped 4K substituted", clientMediaCaps{Explicit: true, MaxHeight: 1080}, []media.Version{uhd, fhd}, 20},
		{"compatible 4K retained", clientMediaCaps{Explicit: true, MaxHeight: 2160, VideoCodecs: map[string]bool{"hevc": true}}, []media.Version{uhd, fhd}, 10},
		{"missing codecs fail closed", clientMediaCaps{Explicit: true, MaxHeight: 2160}, []media.Version{uhd}, 0},
		{"legacy clients unchanged", clientMediaCaps{}, []media.Version{uhd}, 10},
		{"known SDR display hides HDR", clientMediaCaps{Explicit: true, MaxHeight: 2160, VideoCodecs: map[string]bool{"hevc": true}, HDRKnown: true}, []media.Version{{ID: 10, Width: 3840, Height: 2160, VideoCodec: "hevc", HDR: "hdr10", TechnicalStatus: "ok"}}, 0},
		{"unprobed ordinary title retained", clientMediaCaps{Explicit: true, MaxHeight: 1080}, []media.Version{{ID: 10, TechnicalStatus: "pending"}}, 10},
		{"unprobed filename UHD hidden", clientMediaCaps{Explicit: true, MaxHeight: 1080}, []media.Version{{ID: 10, Label: "4K", TechnicalStatus: "pending"}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, ok, _ := selectCatalogVersion(tc.caps, 10, tc.versions)
			if ok != (tc.want > 0) || ok && selected.ID != tc.want {
				t.Fatalf("selected=%+v ok=%v want=%d", selected, ok, tc.want)
			}
		})
	}
}

func TestAdaptCatalogPreservesMetadataAndDeduplicatesAlternate(t *testing.T) {
	caps := clientMediaCaps{Explicit: true, MaxHeight: 1080}
	versions := []media.Version{{ID: 10, Width: 3840, Height: 2160, TechnicalStatus: "ok"}, {ID: 20, LibraryID: 2, Extension: ".mp4", Height: 1080, TechnicalStatus: "ok"}}
	items := []media.Item{{ID: 10, Title: "Filme", PosterURL: "/poster", PositionSeconds: 123}, {ID: 20, Title: "Filme"}}
	out, _ := adaptItemsWithCandidates(caps, items, map[int64][]media.Version{10: versions, 20: versions})
	if len(out) != 1 || out[0].ID != 20 || out[0].LibraryID != 2 || out[0].Title != "Filme" || out[0].PosterURL != "/poster" || out[0].PositionSeconds != 123 {
		t.Fatalf("bad adapted cards: %+v", out)
	}
	if items[0].ID != 10 {
		t.Fatal("shared catalog cache must not be mutated")
	}
	if len(keepUHDItems(out, map[int64][]media.Version{10: versions})) != 0 {
		t.Fatal("FHD alternate must not be relabelled as a UHD shelf item")
	}
}

func TestCPU4KGuardIgnoresCompiledEncodersWithoutDevices(t *testing.T) {
	base := playback.Plan{Available: true, Mode: playback.ModeVideoTranscode, VideoTranscode: true, VideoWidth: 3840, VideoHeight: 1600, VideoCodec: "h264"}
	blocked := applyCPU4KTranscodeBlock(base, transcode.EngineStatus{VideoEncoders: []string{"h264_nvenc", "h264_qsv", "h264_vaapi", "libx264"}})
	if blocked.Available {
		t.Fatal("compiled FFmpeg hardware encoders do not prove a GPU is installed")
	}
	for _, mode := range []string{playback.ModeDirectPlay, playback.ModeAudioCompatibility, playback.ModeRemux, playback.ModeLocalDecode} {
		copyPlan := base
		copyPlan.Mode = mode
		copyPlan.VideoTranscode = false
		if got := applyCPU4KTranscodeBlock(copyPlan, transcode.EngineStatus{}); !got.Available || got.Mode != mode {
			t.Fatalf("4K video-copy route blocked: %+v", got)
		}
	}
	base.VideoWidth = 1920
	base.VideoHeight = 1080
	if !applyCPU4KTranscodeBlock(base, transcode.EngineStatus{}).Available {
		t.Fatal("ordinary FHD CPU transcode must remain available")
	}
}
