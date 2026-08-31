package playback

// DecideForClient preserves the generic deterministic planner while allowing
// native Media3 clients to keep the original multi-audio source when they can
// select a decoder-supported track locally. This is intentionally narrower
// than a codec fallback: video/container/resolution/HDR policy must already be
// valid, and no video transcoding is introduced.
func DecideForClient(source Source, request Request) (plan Plan) {
	plan = Decide(source, request)
	defer func() { plan = applyAutomatic4KCostGuard(plan) }()
	if !request.Capabilities.NativeAudioTrackSelection || plan.Mode == ModeDirectPlay {
		return plan
	}
	if plan.ReasonCode != "audio_codec_unsupported" && plan.ReasonCode != "audio_only_compatibility" {
		return plan
	}
	if !supports(request.Capabilities.Containers, source.Container, normalizeContainer) {
		return plan
	}
	video, ok := firstStream(source.Streams, "video")
	if !ok || !supports(request.Capabilities.VideoCodecs, video.Codec, normalizeCodec) {
		return plan
	}
	audios := streamsOfType(source.Streams, "audio")
	if len(audios) == 0 {
		return plan
	}
	for _, audio := range audios {
		if !supports(request.Capabilities.AudioCodecs, audio.Codec, normalizeCodec) {
			continue
		}
		plan.Available = true
		plan.Mode = ModeDirectPlay
		plan.ReasonCode = "native_audio_track_selection"
		plan.Reason = "the native client can keep the original multi-audio source and select a decoder-supported audio track locally"
		plan.Container = normalizeContainer(source.Container)
		plan.AudioStream = -1
		plan.AudioCodec = ""
		plan.SourceAudioCodec = ""
		plan.AudioLanguage = ""
		plan.AudioTitle = ""
		plan.AudioTranscode = false
		plan.VideoTranscode = false
		plan.ClientSelectsAudio = true
		return plan
	}
	return plan
}