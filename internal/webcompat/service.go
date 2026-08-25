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
	Index     int               `json:"index"`
	CodecName string            `json:"codec_name"`
	CodecType string            `json:"codec_type"`
	Tags      map[string]string `json:"tags"`
}

type Plan struct {
	Available      bool   `json:"available"`
	Mode           string `json:"mode"`
	Reason         string `json:"reason"`
	Container      string `json:"container"`
	VideoCodec     string `json:"video_codec"`
	AudioCodec     string `json:"audio_codec"`
	VideoStream    int    `json:"video_stream"`
	AudioStream    int    `json:"audio_stream"`
	Confidence     string `json:"confidence"`
	FFmpegAvailable bool  `json:"ffmpeg_available"`
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
		"-show_entries", "stream=index,codec_name,codec_type:stream_tags=language,title",
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
		plan.Reason = "video codec " + plan.VideoCodec + " cannot be made browser-compatible by remux alone"
		return plan, nil
	}

	if len(audios) > 0 {
		preferred := pickAudio(audios)
		if preferred.Index < 0 {
			plan.AudioCodec = strings.ToLower(audios[0].CodecName)
			plan.Reason = "no browser-copy-compatible audio track was found; audio codec is " + plan.AudioCodec
			return plan, nil
		}
		plan.AudioStream = preferred.Index
		plan.AudioCodec = strings.ToLower(preferred.CodecName)
	}

	plan.Available = true
	plan.Mode = "remux"
	plan.Confidence = videoConfidence
	if videoConfidence == "safe" {
		plan.Reason = "container can be repackaged to fragmented MP4 without transcoding"
	} else {
		plan.Reason = "remux can be attempted, but this video codec still depends on browser/OS hardware support"
	}
	return plan, nil
}

func pickAudio(streams []StreamInfo) StreamInfo {
	for _, codec := range []string{"aac", "mp3", "eac3", "ac3"} {
		for _, stream := range streams {
			if strings.EqualFold(stream.CodecName, codec) {
				return stream
			}
		}
	}
	return StreamInfo{Index: -1}
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
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
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
