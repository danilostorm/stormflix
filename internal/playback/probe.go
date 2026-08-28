package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type probePayload struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		Index       int               `json:"index"`
		CodecName   string            `json:"codec_name"`
		CodecType   string            `json:"codec_type"`
		Tags        map[string]string `json:"tags"`
		Disposition map[string]int    `json:"disposition"`
	} `json:"streams"`
}

func Probe(ctx context.Context, path string) (Source, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return Source{}, errors.New("ffprobe is not installed in this StormFlix server")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, ffprobe,
		"-v", "error",
		"-show_entries", "format=format_name,duration:stream=index,codec_name,codec_type:stream_tags=language,title:stream_disposition=default",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return Source{}, errors.New("ffprobe timed out while reading the mounted media")
		}
		return Source{}, fmt.Errorf("ffprobe failed: %w", err)
	}
	var payload probePayload
	if err := json.Unmarshal(out, &payload); err != nil {
		return Source{}, fmt.Errorf("decode ffprobe output: %w", err)
	}

	container := normalizeContainer(filepath.Ext(path))
	if container == "" {
		container = normalizeContainer(payload.Format.FormatName)
	}
	duration, _ := strconv.ParseFloat(strings.TrimSpace(payload.Format.Duration), 64)
	source := Source{Container: container, DurationSeconds: duration, Streams: make([]Stream, 0, len(payload.Streams))}
	for _, stream := range payload.Streams {
		kind := strings.ToLower(strings.TrimSpace(stream.CodecType))
		if kind != "video" && kind != "audio" && kind != "subtitle" {
			continue
		}
		source.Streams = append(source.Streams, Stream{
			Index:    stream.Index,
			Type:     kind,
			Codec:    normalizeCodec(stream.CodecName),
			Language: strings.TrimSpace(stream.Tags["language"]),
			Title:    strings.TrimSpace(stream.Tags["title"]),
			Default:  stream.Disposition != nil && stream.Disposition["default"] > 0,
		})
	}
	return source, nil
}
