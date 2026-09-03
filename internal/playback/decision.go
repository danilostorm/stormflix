package playback

import (
	"strconv"
	"strings"
)

func Decide(source Source, request Request) Plan {
	plan := Plan{
		Mode:              ModeUnsupported,
		ClientKind:        strings.ToLower(strings.TrimSpace(request.ClientKind)),
		SourceContainer:   normalizeContainer(source.Container),
		Container:         normalizeContainer(source.Container),
		VideoStream:       -1,
		AudioStream:       -1,
		SourceBitrateKbps: source.BitrateKbps,
		Quality:           normalizeQuality(request.Quality),
	}

	video, ok := firstStream(source.Streams, "video")
	if !ok {
		plan.ReasonCode = "no_video_stream"
		plan.Reason = "no video stream was found"
		return plan
	}
	plan.VideoStream = video.Index
	plan.SourceVideoCodec = normalizeCodec(video.Codec)
	plan.VideoCodec = plan.SourceVideoCodec
	plan.VideoWidth = video.Width
	plan.VideoHeight = video.Height
	plan.VideoFrameRate = video.FrameRate
	plan.VideoHDR = strings.ToLower(strings.TrimSpace(video.HDR))
	plan.AvailableQualities = availableQualities(video.Height)

	audios := streamsOfType(source.Streams, "audio")
	plan.AudioTrackCount = len(audios)
	selectedAudioDefault := true
	if len(audios) > 0 {
		audio := pickAudio(audios, request.PreferredAudioLanguage)
		selectedAudioDefault = audio.Default
		plan.AudioStream = audio.Index
		plan.SourceAudioCodec = normalizeCodec(audio.Codec)
		plan.AudioCodec = plan.SourceAudioCodec
		plan.AudioLanguage = strings.TrimSpace(audio.Language)
		plan.AudioTitle = strings.TrimSpace(audio.Title)
	}

	if code, reason := videoCompatibilityIssue(source, video, request); code != "" {
		if local, ok := localDecodePlan(plan, source, video, request, code, reason); ok {
			return local
		}
		return videoTranscodeOrUnsupported(plan, request, code, reason)
	}

	containerSupported := supports(request.Capabilities.Containers, plan.SourceContainer, normalizeContainer)
	audioSupported := plan.AudioStream < 0 || supports(request.Capabilities.AudioCodecs, plan.SourceAudioCodec, normalizeCodec)
	needsServerAudioSelection := request.Capabilities.ServerSelectsAudio && len(audios) > 1 && !selectedAudioDefault

	if containerSupported && audioSupported && !needsServerAudioSelection {
		plan.Available = true
		plan.Mode = ModeDirectPlay
		plan.ReasonCode = "direct_play_supported"
		plan.Reason = "container, video and selected audio are supported by this client"
		return plan
	}

	if !audioSupported {
		return audioCompatibilityOrUnsupported(plan, request, "selected audio codec "+plan.SourceAudioCodec+" is not supported")
	}

	if needsServerAudioSelection {
		if !request.Capabilities.AllowRemux || !supports(request.Capabilities.Containers, "mp4", normalizeContainer) {
			plan.ReasonCode = "audio_track_selection_unavailable"
			plan.Reason = "the preferred audio track is not the default track and this client requires server-side audio selection, but remux is unavailable"
			return plan
		}
		if plan.AudioStream >= 0 && !mp4AudioCopyCompatible(plan.SourceAudioCodec) {
			return audioCompatibilityOrUnsupported(plan, request, "the preferred server-selected audio track cannot be safely copied into the MP4 compatibility container")
		}
		plan.Available = true
		plan.Mode = ModeRemux
		plan.Container = "mp4"
		plan.ReasonCode = "server_audio_track_selection"
		plan.Reason = "the preferred audio track will be selected server-side while video and audio are copied without re-encoding"
		return plan
	}

	if !containerSupported && request.Capabilities.AllowRemux && supports(request.Capabilities.Containers, "mp4", normalizeContainer) {
		if plan.AudioStream >= 0 && !mp4AudioCopyCompatible(plan.SourceAudioCodec) {
			return audioCompatibilityOrUnsupported(plan, request, "the selected audio track cannot be safely copied into the MP4 compatibility container")
		}
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

func localDecodePlan(plan Plan, source Source, video Stream, request Request, code, reason string) (Plan, bool) {
	// v6 local decode is intentionally a codec fallback, not a way to ignore an
	// explicit quality/bitrate/device limit. Those still require server-side
	// adaptation through the normal PlaybackPlan transcode path.
	if code != "video_codec_unsupported" {
		return plan, false
	}
	codec := normalizeCodec(video.Codec)
	if codec != "hevc" && codec != "av1" {
		return plan, false
	}
	if qh := qualityHeight(request.Quality); qh > 0 && video.Height > qh {
		return plan, false
	}
	if request.Capabilities.DirectPlayMaxBitrateKbps > 0 && source.BitrateKbps > request.Capabilities.DirectPlayMaxBitrateKbps {
		return plan, false
	}
	if !request.LocalDecode.SupportsLocalDecode(codec, video.Width, video.Height, video.HDR) {
		return plan, false
	}

	plan.Available = true
	plan.Mode = ModeLocalDecode
	plan.Container = "mp4"
	plan.LocalDecode = true
	plan.LocalDecodeCodec = codec
	plan.LocalDecodeEngine = "stormflix-v6-wasm"
	plan.VideoTranscode = false
	plan.VideoCodec = codec
	plan.ReasonCode = "local_decode_" + codec
	plan.Reason = reason + "; StormFlix will keep the source video bitstream and decode it on this device"

	// The current HEVC WASM/HLS pipeline only guarantees AAC pass-through for
	// muxed audio. Convert audio only when required; video always stays copy.
	if plan.AudioStream >= 0 && plan.SourceAudioCodec != "aac" {
		if request.Capabilities.AllowAudioCompatibility && supports(request.Capabilities.AudioCodecs, "aac", normalizeCodec) {
			plan.AudioCodec = "aac"
			plan.AudioTranscode = true
			plan.TranscodeReasons = append(plan.TranscodeReasons, "audio_aac_compatibility")
		} else {
			return Plan{}, false
		}
	}
	return plan, true
}

func videoCompatibilityIssue(source Source, video Stream, request Request) (string, string) {
	codec := normalizeCodec(video.Codec)
	if !supports(request.Capabilities.VideoCodecs, codec, normalizeCodec) {
		return "video_codec_unsupported", "video codec " + codec + " is not supported natively by this client"
	}
	if profile, exists := videoProfileFor(request.Capabilities.VideoProfiles, codec); exists {
		if profile.MaxWidth > 0 && video.Width > profile.MaxWidth {
			return "video_resolution_unsupported", "video width exceeds this client's advertised decode profile"
		}
		if profile.MaxHeight > 0 && video.Height > profile.MaxHeight {
			return "video_resolution_unsupported", "video height exceeds this client's advertised decode profile"
		}
		if profile.MaxFrameRate > 0 && video.FrameRate > profile.MaxFrameRate+0.01 {
			return "video_framerate_unsupported", "video frame rate exceeds this client's advertised decode profile"
		}
		hdr := strings.ToLower(strings.TrimSpace(video.HDR))
		if profile.HDRKnown && hdr != "" && !containsNormalized(profile.HDRTypes, hdr) {
			return "video_hdr_unsupported", "the source HDR format is not supported by this client's advertised decode profile"
		}
	}
	if qh := qualityHeight(request.Quality); qh > 0 && video.Height > qh {
		return "quality_limit", "the selected playback quality is lower than the source resolution"
	}
	if request.Capabilities.DirectPlayMaxBitrateKbps > 0 && source.BitrateKbps > request.Capabilities.DirectPlayMaxBitrateKbps {
		return "direct_play_bitrate_limit", "source bitrate exceeds the client's explicit Direct Play limit"
	}
	return "", ""
}

func videoTranscodeOrUnsupported(plan Plan, request Request, code, reason string) Plan {
	if !request.Capabilities.AllowVideoTranscode {
		plan.ReasonCode = code
		plan.Reason = reason + "; video transcoding is unavailable for this client"
		return plan
	}
	targetCodec := chooseTranscodeVideoCodec(request.Capabilities.VideoCodecs)
	if targetCodec == "" {
		plan.ReasonCode = "video_transcode_target_unavailable"
		plan.Reason = reason + "; the client did not advertise a safe video output codec"
		return plan
	}

	plan.Available = true
	plan.Mode = ModeVideoTranscode
	plan.Container = "mp4"
	plan.VideoTranscode = true
	plan.VideoCodec = targetCodec
	plan.ReasonCode = code
	plan.TranscodeReasons = []string{code}
	plan.Reason = reason + "; StormFlix will transcode only what is required for this device"

	profile, _ := videoProfileFor(request.Capabilities.VideoProfiles, targetCodec)
	plan.TargetVideoWidth, plan.TargetVideoHeight = targetDimensions(plan.VideoWidth, plan.VideoHeight, profile, plan.Quality)
	plan.TargetVideoFrameRate = plan.VideoFrameRate
	if profile.MaxFrameRate > 0 && plan.TargetVideoFrameRate > profile.MaxFrameRate {
		plan.TargetVideoFrameRate = profile.MaxFrameRate
	}
	plan.TargetBitrateKbps = targetBitrate(plan.TargetVideoHeight, request.Capabilities.MaxTranscodeBitrateKbps, request.Capabilities.DirectPlayMaxBitrateKbps)
	plan.ToneMap = shouldToneMap(plan.VideoHDR, targetCodec, profile)
	if plan.ToneMap {
		plan.TranscodeReasons = append(plan.TranscodeReasons, "tone_map_sdr")
	}

	if plan.AudioStream >= 0 {
		if supports(request.Capabilities.AudioCodecs, plan.SourceAudioCodec, normalizeCodec) && mp4AudioCopyCompatible(plan.SourceAudioCodec) {
			plan.AudioCodec = plan.SourceAudioCodec
		} else if supports(request.Capabilities.AudioCodecs, "aac", normalizeCodec) || request.Capabilities.AllowAudioCompatibility {
			plan.AudioCodec = "aac"
			plan.AudioTranscode = true
			plan.TranscodeReasons = append(plan.TranscodeReasons, "audio_aac_compatibility")
		} else {
			plan.Available = false
			plan.Mode = ModeUnsupported
			plan.ReasonCode = "video_transcode_audio_target_unavailable"
			plan.Reason = "video can be transcoded, but no compatible audio output codec is available"
			return plan
		}
	}
	return plan
}

func chooseTranscodeVideoCodec(codecs []string) string {
	for _, preferred := range []string{"h264", "hevc", "av1"} {
		if supports(codecs, preferred, normalizeCodec) {
			return preferred
		}
	}
	return ""
}

func targetDimensions(width, height int, profile VideoProfile, quality string) (int, int) {
	if width <= 0 || height <= 0 {
		return width, height
	}
	maxW, maxH := profile.MaxWidth, profile.MaxHeight
	if qh := qualityHeight(quality); qh > 0 && (maxH == 0 || qh < maxH) {
		maxH = qh
	}
	if maxW == 0 && maxH == 0 {
		return even(width), even(height)
	}
	scale := 1.0
	if maxW > 0 && width > maxW {
		scale = minFloat(scale, float64(maxW)/float64(width))
	}
	if maxH > 0 && height > maxH {
		scale = minFloat(scale, float64(maxH)/float64(height))
	}
	if scale >= 1 {
		return even(width), even(height)
	}
	return even(int(float64(width) * scale)), even(int(float64(height) * scale))
}

func even(value int) int {
	if value <= 0 {
		return value
	}
	if value%2 != 0 {
		value--
	}
	if value < 2 {
		return 2
	}
	return value
}

func targetBitrate(height int, caps ...int64) int64 {
	var bitrate int64
	switch {
	case height >= 2160:
		bitrate = 20000
	case height >= 1440:
		bitrate = 12000
	case height >= 1080:
		bitrate = 8000
	case height >= 720:
		bitrate = 4500
	case height >= 480:
		bitrate = 2500
	default:
		bitrate = 1500
	}
	for _, cap := range caps {
		if cap > 0 && cap < bitrate {
			bitrate = cap
		}
	}
	if bitrate < 500 {
		bitrate = 500
	}
	return bitrate
}

func shouldToneMap(sourceHDR, targetCodec string, profile VideoProfile) bool {
	hdr := strings.ToLower(strings.TrimSpace(sourceHDR))
	if hdr == "" || hdr == "sdr" {
		return false
	}
	if profile.HDRKnown && containsNormalized(profile.HDRTypes, hdr) {
		return false
	}
	return normalizeCodec(targetCodec) == "h264" || profile.HDRKnown
}

func availableQualities(height int) []string {
	qualities := []string{"auto", "original"}
	for _, option := range []struct {
		height int
		value  string
	}{
		{2160, "2160p"},
		{1440, "1440p"},
		{1080, "1080p"},
		{720, "720p"},
		{480, "480p"},
	} {
		if height >= option.height {
			qualities = append(qualities, option.value)
		}
	}
	return qualities
}

func normalizeQuality(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "p")
	switch value {
	case "2160", "4k", "uhd":
		return "2160p"
	case "1440", "2k":
		return "1440p"
	case "1080", "fullhd", "fhd":
		return "1080p"
	case "720", "hd":
		return "720p"
	case "480", "sd":
		return "480p"
	case "original":
		return "original"
	default:
		return "auto"
	}
}

func qualityHeight(value string) int {
	value = normalizeQuality(value)
	if value == "auto" || value == "original" {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(value, "p"))
	return n
}

func minFloat(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func audioCompatibilityOrUnsupported(plan Plan, request Request, reason string) Plan {
	if request.Capabilities.AllowAudioCompatibility && supports(request.Capabilities.AudioCodecs, "aac", normalizeCodec) && supports(request.Capabilities.Containers, "mp4", normalizeContainer) {
		plan.Available = true
		plan.Mode = ModeAudioCompatibility
		plan.Container = "mp4"
		plan.AudioCodec = "aac"
		plan.AudioTranscode = true
		plan.ReasonCode = "audio_only_compatibility"
		plan.Reason = reason + "; video will be copied without re-encoding and selected audio will be converted to AAC"
		return plan
	}
	plan.ReasonCode = "audio_codec_unsupported"
	plan.Reason = reason + " and audio-only compatibility is unavailable"
	return plan
}

func videoProfileFor(profiles []VideoProfile, codec string) (VideoProfile, bool) {
	codec = normalizeCodec(codec)
	for _, profile := range profiles {
		if normalizeCodec(profile.Codec) == codec {
			return profile, true
		}
	}
	return VideoProfile{}, false
}

func containsNormalized(values []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == wanted {
			return true
		}
	}
	return false
}

func mp4AudioCopyCompatible(codec string) bool {
	switch normalizeCodec(codec) {
	case "aac", "eac3", "ac3", "mp3":
		return true
	default:
		return false
	}
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
