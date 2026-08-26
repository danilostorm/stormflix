package webcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type StreamInfo struct {
	Index       int               `json:"index"`
	CodecName   string            `json:"codec_name"`
	CodecType   string            `json:"codec_type"`
	Tags        map[string]string `json:"tags"`
	Disposition map[string]int    `json:"disposition"`
}

type Plan struct {
	Available        bool   `json:"available"`
	Mode             string `json:"mode"`
	Reason           string `json:"reason"`
	Container        string `json:"container"`
	VideoCodec       string `json:"video_codec"`
	AudioCodec       string `json:"audio_codec"`
	SourceAudioCodec string `json:"source_audio_codec,omitempty"`
	AudioLanguage    string `json:"audio_language,omitempty"`
	AudioTitle       string `json:"audio_title,omitempty"`
	AudioTrackCount  int    `json:"audio_track_count"`
	VideoStream      int    `json:"video_stream"`
	AudioStream      int    `json:"audio_stream"`
	AudioTranscode   bool   `json:"audio_transcode"`
	Confidence       string `json:"confidence"`
	FFmpegAvailable  bool   `json:"ffmpeg_available"`
}

type probeOutput struct {
	Streams []StreamInfo `json:"streams"`
}

func Probe(ctx context.Context, path string) (Plan, error) {
	plan := Plan{Mode: "unsupported", VideoStream: -1, AudioStream: -1, Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		plan.Reason = "ffprobe is not installed in this StormFlix server"
		return plan, nil
	}
	plan.FFmpegAvailable = true

	probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, ffprobe,
		"-v", "error",
		"-show_entries", "stream=index,codec_name,codec_type:stream_tags=language,title:stream_disposition=default",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return plan, errors.New("ffprobe timed out while reading the mounted media")
		}
		return plan, fmt.Errorf("ffprobe failed: %w", err)
	}
	var data probeOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return plan, fmt.Errorf("decode ffprobe output: %w", err)
	}

	var videos []StreamInfo
	var audios []StreamInfo
	for _, stream := range data.Streams {
		switch strings.ToLower(stream.CodecType) {
		case "video":
			videos = append(videos, stream)
		case "audio":
			audios = append(audios, stream)
		}
	}
	plan.AudioTrackCount = len(audios)
	if len(videos) == 0 {
		plan.Reason = "no video stream was found"
		return plan, nil
	}
	video := videos[0]
	plan.VideoStream = video.Index
	plan.VideoCodec = strings.ToLower(video.CodecName)

	videoConfidence := ""
	switch plan.VideoCodec {
	case "h264", "avc1":
		videoConfidence = "safe"
	case "hevc", "h265", "av1":
		videoConfidence = "conditional"
	default:
		plan.Reason = "video codec " + plan.VideoCodec + " cannot be made compatible without video transcoding"
		return plan, nil
	}

	if len(audios) > 0 {
		// Language/default-track preference comes before codec convenience. This
		// prevents an English AAC track from replacing the preferred/default
		// Portuguese track merely because the latter needs audio-only conversion.
		preferred := pickAnyAudio(audios)
		if preferred.Index < 0 {
			plan.Reason = "no usable audio track was found"
			return plan, nil
		}

		plan.AudioStream = preferred.Index
		plan.SourceAudioCodec = strings.ToLower(preferred.CodecName)
		plan.AudioLanguage = strings.TrimSpace(preferred.Tags["language"])
		plan.AudioTitle = strings.TrimSpace(preferred.Tags["title"])
		if !isCopyCompatibleAudio(preferred.CodecName) {
			plan.AudioTranscode = true
			plan.AudioCodec = "aac"
		} else {
			plan.AudioCodec = plan.SourceAudioCodec
		}
	}

	plan.Available = true
	plan.Mode = "remux"
	plan.Confidence = videoConfidence
	if plan.AudioTranscode {
		plan.Reason = "video will be copied without re-encoding; the preferred audio track will be converted to AAC for device compatibility"
	} else if videoConfidence == "safe" {
		plan.Reason = "container can be repackaged to MP4 without video transcoding"
	} else {
		plan.Reason = "remux can be attempted, but this video codec still depends on browser/OS hardware support"
	}
	return plan, nil
}

// Kept for unit tests and callers that explicitly need a copy-compatible audio
// choice. Playback planning itself is preference-first via pickAnyAudio.
func pickAudio(streams []StreamInfo) StreamInfo {
	best := StreamInfo{Index: -1}
	bestScore := -1
	for _, stream := range streams {
		score := audioCopyScore(stream)
		if score > bestScore {
			best = stream
			bestScore = score
		}
	}
	if bestScore < 0 {
		return StreamInfo{Index: -1}
	}
	return best
}

func pickAnyAudio(streams []StreamInfo) StreamInfo {
	best := StreamInfo{Index: -1}
	bestScore := -1
	for _, stream := range streams {
		score := audioLanguageScore(stream) + audioCodecPreference(stream.CodecName)
		if score > bestScore {
			best = stream
			bestScore = score
		}
	}
	return best
}

func audioCopyScore(stream StreamInfo) int {
	if !isCopyCompatibleAudio(stream.CodecName) {
		return -1
	}
	return audioLanguageScore(stream) + audioCodecPreference(stream.CodecName)
}

func isCopyCompatibleAudio(codecName string) bool {
	switch strings.ToLower(strings.TrimSpace(codecName)) {
	case "aac", "eac3", "ac3", "mp3":
		return true
	default:
		return false
	}
}

func audioCodecPreference(codecName string) int {
	switch strings.ToLower(strings.TrimSpace(codecName)) {
	case "aac":
		return 40
	case "eac3":
		return 35
	case "ac3":
		return 30
	case "mp3":
		return 20
	case "truehd", "dts", "dca", "dtshd", "flac", "opus", "vorbis":
		return 10
	default:
		return 1
	}
}

func audioLanguageScore(stream StreamInfo) int {
	language := strings.ToLower(strings.TrimSpace(stream.Tags["language"]))
	title := strings.ToLower(strings.TrimSpace(stream.Tags["title"]))
	score := 0
	if stream.Disposition != nil && stream.Disposition["default"] > 0 {
		score += 80
	}
	if language == "pt-br" || language == "pt_br" || language == "pob" {
		return score + 160
	}
	if language == "pt" || language == "por" || language == "por-br" {
		return score + 150
	}
	if strings.Contains(title, "portugu") || strings.Contains(title, "pt-br") || strings.Contains(title, "pt br") || strings.Contains(title, "dublado") || strings.Contains(title, "brasil") || strings.Contains(title, "brazil") {
		return score + 140
	}
	return score
}

func Stream(ctx context.Context, path string, plan Plan, dst io.Writer) error {
	if !plan.Available || plan.VideoStream < 0 {
		return errors.New("web remux is not available for this media")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg is not installed")
	}

	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", path,
		"-map", fmt.Sprintf("0:%d", plan.VideoStream),
	}
	if plan.AudioStream >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", plan.AudioStream))
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")
	if plan.AudioTranscode && plan.AudioStream >= 0 {
		args = append(args,
			"-c:v", "copy",
			"-c:a", "aac",
			"-b:a", "256k",
			"-ac", "2",
			"-ar", "48000",
		)
	} else {
		args = append(args, "-c", "copy")
	}
	if plan.VideoCodec == "hevc" || plan.VideoCodec == "h265" {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args,
		"-movflags", "+frag_keyframe+empty_moov+default_base_moof",
		"-avoid_negative_ts", "make_zero",
		"-f", "mp4", "pipe:1",
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stdout = dst
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 600 {
			msg = msg[:600]
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ffmpeg remux failed: %s", msg)
	}
	return nil
}
