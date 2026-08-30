package playback

import "testing"

func TestApplyAudioStreamForcesServerSelectionForNonDefaultTrack(t *testing.T) {
	source := Source{Container: "mp4", DurationSeconds: 120, Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "aac", Language: "eng", Default: true},
		{Index: 2, Type: "audio", Codec: "aac", Language: "por"},
	}}
	// Be explicit here: the normal planner intentionally prefers Portuguese
	// tracks when no profile language is supplied, even when they are not the
	// container default. This test needs an initial Direct Play route so it can
	// verify the transition caused specifically by choosing stream 2.
	req := Request{ClientKind: "web", PreferredAudioLanguage: "eng", Capabilities: Capabilities{
		Containers: []string{"mp4"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"},
		AllowRemux: true, AllowAudioCompatibility: true, AllowVideoTranscode: true, ServerSelectsAudio: true,
	}}
	base := Decide(source, req)
	if base.Mode != ModeDirectPlay {
		t.Fatalf("explicit preferred default source should direct play, got %s", base.Mode)
	}
	plan := ApplyAudioStream(source, req, base, 2)
	if !plan.Available || plan.Mode != ModeRemux {
		t.Fatalf("non-default browser track should be server-selected through remux, got available=%v mode=%s reason=%s", plan.Available, plan.Mode, plan.Reason)
	}
	if plan.AudioStream != 2 || plan.AudioLanguage != "por" || !plan.ClientSelectsAudio {
		t.Fatalf("wrong selected audio metadata: %+v", plan)
	}
}

func TestApplyAudioStreamUsesAACWhenSelectedTrackCannotBeCopied(t *testing.T) {
	source := Source{Container: "mkv", DurationSeconds: 120, Streams: []Stream{
		{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Type: "audio", Codec: "dts", Language: "por", Default: true},
	}}
	req := Request{ClientKind: "web", Capabilities: Capabilities{
		Containers: []string{"mp4"}, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"},
		AllowRemux: true, AllowAudioCompatibility: true, AllowVideoTranscode: true, ServerSelectsAudio: true,
	}}
	plan := ApplyAudioStream(source, req, Decide(source, req), 1)
	if !plan.Available || plan.Mode != ModeAudioCompatibility || !plan.AudioTranscode || plan.AudioCodec != "aac" {
		t.Fatalf("DTS browser track should keep video copy and use AAC, got %+v", plan)
	}
}
