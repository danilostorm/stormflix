package webcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type DetailedStream struct {
	Index      int               `json:"index"`
	CodecName  string            `json:"codec_name"`
	CodecType  string            `json:"codec_type"`
	Width      int               `json:"width,omitempty"`
	Height     int               `json:"height,omitempty"`
	Channels   int               `json:"channels,omitempty"`
	SampleRate string            `json:"sample_rate,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// DetailedStreams is intentionally read-only ffprobe metadata. It is shared by
// compatibility clients (Jellyfin layer and diagnostics) and never transcodes.
func DetailedStreams(ctx context.Context, path string) ([]DetailedStream, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, errors.New("ffprobe is not installed")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, ffprobe,
		"-v", "error",
		"-show_entries", "stream=index,codec_name,codec_type,width,height,channels,sample_rate:stream_tags=language,title",
		"-of", "json", path,
	)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("ffprobe timed out")
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	var payload struct{ Streams []DetailedStream `json:"streams"` }
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	return payload.Streams, nil
}
