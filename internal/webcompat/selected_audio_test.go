package webcompat

import "testing"

func TestApplySelectedAudioKeepsAACTrack(t *testing.T) {
	plan := Plan{Available: true, VideoStream: 0, AudioStream: 1, AudioCodec: "dts", SourceAudioCodec: "dts", AudioTranscode: true}
	applySelectedAudio(&plan, StreamInfo{Index: 4, CodecName: "aac", CodecType: "audio", Tags: map[string]string{"language": "eng", "title": "English AAC"}})
	if plan.AudioStream != 4 || plan.AudioCodec != "aac" || plan.SourceAudioCodec != "aac" || plan.AudioTranscode {
		t.Fatalf("unexpected selected AAC plan: %+v", plan)
	}
	if plan.AudioLanguage != "eng" || plan.AudioTitle != "English AAC" {
		t.Fatalf("selected audio metadata was not retained: %+v", plan)
	}
}

func TestApplySelectedAudioTranscodesOnlyUnsupportedAudio(t *testing.T) {
	plan := Plan{Available: true, VideoStream: 0}
	applySelectedAudio(&plan, StreamInfo{Index: 7, CodecName: "truehd", CodecType: "audio", Tags: map[string]string{"language": "jpn"}})
	if plan.AudioStream != 7 || plan.AudioCodec != "aac" || plan.SourceAudioCodec != "truehd" || !plan.AudioTranscode {
		t.Fatalf("unexpected selected TrueHD plan: %+v", plan)
	}
}
