package webcompat

import "testing"

func TestPickAudioPrefersAAC(t *testing.T) {
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

func TestPickAudioRejectsCopyIncompatibleAudio(t *testing.T) {
	got := pickAudio([]StreamInfo{{Index: 4, CodecName: "truehd", CodecType: "audio"}})
	if got.Index != -1 {
		t.Fatalf("expected no compatible audio stream, got %+v", got)
	}
}
