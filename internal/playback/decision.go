package playback

import "strings"

func Decide(source Source, request Request) Plan {
	plan := Plan{
		Mode:            ModeUnsupported,
		ClientKind:      strings.ToLower(strings.TrimSpace(request.ClientKind)),
		SourceContainer: normalizeContainer(source.Container),
		Container:       normalizeContainer(source.Container),
		VideoStream:     -1,
		AudioStream:     -1,
	}

	video, ok := firstStream(source.Streams, "video")
	if !ok {
		plan.ReasonCode = "no_video_stream"
		plan.Reason = "no video stream was found"
		return plan
	}
	plan.VideoStream = video.Index
	plan.VideoCodec = normalizeCodec(video.Codec)
	if !supports(request.Capabilities.VideoCodecs, plan.VideoCodec, normalizeCodec) {
		plan.ReasonCode = "video_codec_unsupported"
		plan.Reason = "video codec " + plan.VideoCodec + " is not supported by this client; StormFlix will not silently transcode video"
		return plan
	}

	audios := streamsOfType(source.Streams, "audio")
	plan.AudioTrackCount = len(audios)
	if len(audios) > 0 {
		audio := pickAudio(audios, request.PreferredAudioLanguage)
		plan.AudioStream = audio.Index
		plan.SourceAudioCodec = normalizeCodec(audio.Codec)
		plan.AudioCodec = plan.SourceAudioCodec
		plan.AudioLanguage = strings.TrimSpace(audio.Language)
		plan.AudioTitle = strings.TrimSpace(audio.Title)
	}

	containerSupported := supports(request.Capabilities.Containers, plan.SourceContainer, normalizeContainer)
	audioSupported := plan.AudioStream < 0 || supports(request.Capabilities.AudioCodecs, plan.SourceAudioCodec, normalizeCodec)

	if containerSupported && audioSupported {
		plan.Available = true
		plan.Mode = ModeDirectPlay
		plan.ReasonCode = "direct_play_supported"
		plan.Reason = "container, video and selected audio are supported by this client"
		return plan
	}

	if !audioSupported {
		if request.Capabilities.AllowAudioCompatibility && supports(request.Capabilities.AudioCodecs, "aac", normalizeCodec) && supports(request.Capabilities.Containers, "mp4", normalizeContainer) {
			plan.Available = true
			plan.Mode = ModeAudioCompatibility
			plan.Container = "mp4"
			plan.AudioCodec = "aac"
			plan.AudioTranscode = true
			plan.ReasonCode = "audio_only_compatibility"
			plan.Reason = "video is supported and will be copied without re-encoding; selected audio will be converted to AAC"
			return plan
		}
		plan.ReasonCode = "audio_codec_unsupported"
		plan.Reason = "selected audio codec " + plan.SourceAudioCodec + " is not supported and audio-only compatibility is unavailable"
		return plan
	}

	if !containerSupported && request.Capabilities.AllowRemux && supports(request.Capabilities.Containers, "mp4", normalizeContainer) {
		plan.Available = true
		plan.Mode = ModeRemux
		plan.Container = "mp4"
		plan.ReasonCode = "container_remux"
		plan.Reason = "video and selected audio are supported; container will be repackaged to MP4 without re-encoding"
		return plan
	}

	plan.ReasonCode = "container_unsupported"
	plan.Reason = "container " + plan.SourceContainer + " is not supported by this client and remux is unavailable"
	return plan
}

func firstStream(streams []Stream, kind string) (Stream, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, stream := range streams {
		if strings.ToLower(strings.TrimSpace(stream.Type)) == kind {
			return stream, true
		}
	}
	return Stream{}, false
}

func streamsOfType(streams []Stream, kind string) []Stream {
	kind = strings.ToLower(strings.TrimSpace(kind))
	out := make([]Stream, 0)
	for _, stream := range streams {
		if strings.ToLower(strings.TrimSpace(stream.Type)) == kind {
			out = append(out, stream)
		}
	}
	return out
}

func pickAudio(streams []Stream, preferred string) Stream {
	best := streams[0]
	bestScore := -1 << 30
	for _, stream := range streams {
		score := audioLanguageScore(stream, preferred)
		if score > bestScore {
			best = stream
			bestScore = score
		}
	}
	return best
}

func audioLanguageScore(stream Stream, preferred string) int {
	language := normalizeLanguage(stream.Language)
	title := strings.ToLower(strings.TrimSpace(stream.Title))
	preferred = normalizeLanguage(preferred)
	score := 0
	if stream.Default {
		score += 80
	}

	if preferred != "" {
		if language == preferred {
			return score + 300
		}
		if preferred == "pt-br" {
			switch language {
			case "pob":
				return score + 290
			case "pt":
				return score + 280
			case "por":
				return score + 270
			}
			if portugueseTitle(title) {
				return score + 260
			}
		}
		if sameLanguageFamily(language, preferred) {
			return score + 240
		}
	}

	// Preserve StormFlix's established default preference when the profile does
	// not publish one explicitly.
	switch language {
	case "pt-br", "pob":
		return score + 200
	case "pt":
		return score + 190
	case "por":
		return score + 180
	}
	if portugueseTitle(title) {
		return score + 170
	}
	return score
}

func portugueseTitle(title string) bool {
	return strings.Contains(title, "portugu") || strings.Contains(title, "pt-br") || strings.Contains(title, "pt br") || strings.Contains(title, "dublado") || strings.Contains(title, "brasil") || strings.Contains(title, "brazil")
}

func sameLanguageFamily(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if (a == "por" || a == "pob") && strings.HasPrefix(b, "pt") {
		return true
	}
	if (b == "por" || b == "pob") && strings.HasPrefix(a, "pt") {
		return true
	}
	aBase := strings.SplitN(a, "-", 2)[0]
	bBase := strings.SplitN(b, "-", 2)[0]
	return aBase == bBase
}

func supports(values []string, wanted string, normalizer func(string) string) bool {
	wanted = normalizer(wanted)
	if wanted == "" {
		return false
	}
	for _, value := range values {
		if normalizer(value) == wanted {
			return true
		}
	}
	return false
}

func normalizeCodec(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "avc", "avc1":
		return "h264"
	case "h265":
		return "hevc"
	case "dca", "dtshd", "dts-hd", "dts_hd":
		return "dts"
	default:
		return value
	}
}

func normalizeContainer(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, ".")))
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	switch value {
	case "m4v", "mov,mp4,m4a,3gp,3g2,mj2":
		return "mp4"
	case "matroska":
		return "mkv"
	default:
		return value
	}
}

func normalizeLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.TrimSpace(value)
	switch value {
	case "por-br":
		return "pt-br"
	default:
		return value
	}
}
