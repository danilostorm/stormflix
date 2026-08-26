package webcompat

import "testing"

func TestPickAudioPrefersAACWhenLanguageIsEqual(t *testing.T) {
	streams := []StreamInfo{
		{Index: 1, CodecName: "dts", CodecType: "audio"},
		{Index: 2, CodecName: "aac", CodecType: "audio"},
		{Index: 3, CodecName: "eac3", CodecType: "audio"},
	}
	got := pickAudio(streams)
	if got.Index != 2 {
		t.Fatalf("expected AAC stream 2, got %+v", got)
	}
}

func TestPickAudioPrefersPortugueseOverEnglishCodecPriority(t *testing.T) {
	streams := []StreamInfo{
		{Index: 1, CodecName: "aac", CodecType: "audio", Tags: map[string]string{"language": "eng", "title": "English"}},
		{Index: 2, CodecName: "ac3", CodecType: "audio", Tags: map[string]string{"language": "por", "title": "Português 5.1"}},
	}
	got := pickAudio(streams)
	if got.Index != 2 {
		t.Fatalf("expected Portuguese AC3 stream 2, got %+v", got)
	}
}

func TestPickAudioRecognizesDubbedPortugueseTitle(t *testing.T) {
	streams := []StreamInfo{
		{Index: 1, CodecName: "aac", CodecType: "audio", Tags: map[string]string{"language": "eng"}},
		{Index: 2, CodecName: "eac3", CodecType: "audio", Tags: map[string]string{"title": "Dublado Português Brasil"}},
	}
	got := pickAudio(streams)
	if got.Index != 2 {
		t.Fatalf("expected dubbed Portuguese EAC3 stream 2, got %+v", got)
	}
}

func TestPickAudioRejectsCopyIncompatibleAudio(t *testing.T) {
	got := pickAudio([]StreamInfo{{Index: 4, CodecName: "truehd", CodecType: "audio"}})
	if got.Index != -1 {
		t.Fatalf("expected no compatible audio stream, got %+v", got)
	}
}

func TestPickAnyAudioAllowsTrueHDFallback(t *testing.T) {
	got := pickAnyAudio([]StreamInfo{{Index: 4, CodecName: "truehd", CodecType: "audio", Tags: map[string]string{"language": "por"}}})
	if got.Index != 4 {
		t.Fatalf("expected TrueHD stream 4 as transcode fallback, got %+v", got)
	}
}

func TestPickAnyAudioPrefersPortugueseDTSOverEnglishAAC(t *testing.T) {
	streams := []StreamInfo{
		{Index: 1, CodecName: "aac", CodecType: "audio", Tags: map[string]string{"language": "eng"}},
		{Index: 2, CodecName: "dts", CodecType: "audio", Tags: map[string]string{"language": "por", "title": "Português DTS"}},
	}
	got := pickAnyAudio(streams)
	if got.Index != 2 {
		t.Fatalf("expected Portuguese DTS stream 2, got %+v", got)
	}
}
