package webcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type selectedAudioProbe struct {
	Streams []StreamInfo `json:"streams"`
}

// ProbeWithAudioStream keeps Probe's compatibility rules but makes the
// playback planner's selected audio stream authoritative for remux/AAC
// execution. This closes the gap where a non-Portuguese profile could receive
// a correct PlaybackPlan but the legacy remux adapter picked a different track.
func ProbeWithAudioStream(ctx context.Context, path string, audioStream int) (Plan, error) {
	plan, err := Probe(ctx, path)
	if err != nil || audioStream < 0 {
		return plan, err
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return plan, errors.New("ffprobe is not installed")
	}
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
			return plan, errors.New("ffprobe timed out while selecting the audio stream")
		}
		return plan, fmt.Errorf("ffprobe failed while selecting audio stream: %w", err)
	}
	var payload selectedAudioProbe
	if err := json.Unmarshal(out, &payload); err != nil {
		return plan, fmt.Errorf("decode ffprobe output: %w", err)
	}
	for _, stream := range payload.Streams {
		if stream.Index == audioStream && strings.EqualFold(strings.TrimSpace(stream.CodecType), "audio") {
			applySelectedAudio(&plan, stream)
			return plan, nil
		}
	}
	return plan, fmt.Errorf("audio stream %d was not found", audioStream)
}

func applySelectedAudio(plan *Plan, stream StreamInfo) {
	if plan == nil {
		return
	}
	plan.AudioStream = stream.Index
	plan.SourceAudioCodec = strings.ToLower(strings.TrimSpace(stream.CodecName))
	plan.AudioLanguage = strings.TrimSpace(stream.Tags["language"])
	plan.AudioTitle = strings.TrimSpace(stream.Tags["title"])
	if !isCopyCompatibleAudio(stream.CodecName) {
		plan.AudioTranscode = true
		plan.AudioCodec = "aac"
		plan.Reason = "video will be copied without re-encoding; the explicitly selected audio track will be converted to AAC for device compatibility"
		return
	}
	plan.AudioTranscode = false
	plan.AudioCodec = plan.SourceAudioCodec
	plan.Reason = "container can be repackaged while preserving the explicitly selected audio track"
}
