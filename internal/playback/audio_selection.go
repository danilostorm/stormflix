package playback

import "strings"

// ApplyAudioStream replans playback for an exact ffprobe audio stream index.
// Browser HTMLMediaElement does not reliably expose MKV/MP4 audioTracks, so the
// Web client selects a server stream exactly like mature media servers do.
func ApplyAudioStream(source Source, request Request, current Plan, streamIndex int) Plan {
	audios := streamsOfType(source.Streams, "audio")
	var selected Stream
	found := false
	for _, stream := range audios {
		if stream.Index == streamIndex {
			selected = stream
			found = true
			break
		}
	}
	if !found {
		current.Available = false
		current.Mode = ModeUnsupported
		current.ReasonCode = "audio_stream_not_found"
		current.Reason = "the selected audio stream does not exist in this source"
		return current
	}

	// Re-run the normal decision engine with one logical audio stream so every
	// video/quality/HDR decision stays identical. Physical multi-track handling
	// is restored below when the chosen stream is not the source default.
	logical := source
	logical.Streams = make([]Stream, 0, len(source.Streams))
	for _, stream := range source.Streams {
		if strings.EqualFold(strings.TrimSpace(stream.Type), "audio") {
			continue
		}
		logical.Streams = append(logical.Streams, stream)
	}
	logical.Streams = append(logical.Streams, selected)
	plan := Decide(logical, request)
	plan.AudioTrackCount = len(audios)
	plan.AudioStream = selected.Index
	plan.SourceAudioCodec = normalizeCodec(selected.Codec)
	plan.AudioLanguage = strings.TrimSpace(selected.Language)
	plan.AudioTitle = strings.TrimSpace(selected.Title)
	plan.ClientSelectsAudio = true

	if !plan.Available {
		return plan
	}

	// A browser cannot choose a non-default physical track just because the
	// container itself is directly playable. Force server-side stream selection
	// while preserving video stream-copy whenever possible.
	if plan.Mode == ModeDirectPlay && len(audios) > 1 && !selected.Default && request.Capabilities.ServerSelectsAudio {
		if request.Capabilities.AllowRemux && supports(request.Capabilities.Containers, "mp4", normalizeContainer) && mp4AudioCopyCompatible(plan.SourceAudioCodec) {
			plan.Mode = ModeRemux
			plan.Container = "mp4"
			plan.ReasonCode = "server_audio_track_selection"
			plan.Reason = "the selected audio track will be selected server-side while video and audio remain stream-copy"
			return plan
		}
		return audioCompatibilityOrUnsupported(plan, request, "the selected browser audio track requires server-side selection")
	}
	return plan
}
