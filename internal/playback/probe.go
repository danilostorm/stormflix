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
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index        int               `json:"index"`
		CodecName    string            `json:"codec_name"`
		CodecType    string            `json:"codec_type"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		ColorTransfer string           `json:"color_transfer"`
		ColorPrimaries string          `json:"color_primaries"`
		PixelFormat  string            `json:"pix_fmt"`
		BitRate      string            `json:"bit_rate"`
		Tags         map[string]string `json:"tags"`
		Disposition  map[string]int    `json:"disposition"`
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
		"-show_entries", "format=format_name,duration,bit_rate:stream=index,codec_name,codec_type,width,height,avg_frame_rate,r_frame_rate,color_transfer,color_primaries,pix_fmt,bit_rate:stream_tags=language,title:stream_disposition=default",
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
	source := Source{
		Container:       container,
		DurationSeconds: duration,
		BitrateKbps:     parseBitrateKbps(payload.Format.BitRate),
		Streams:         make([]Stream, 0, len(payload.Streams)),
	}
	for _, stream := range payload.Streams {
		kind := strings.ToLower(strings.TrimSpace(stream.CodecType))
		if kind != "video" && kind != "audio" && kind != "subtitle" {
			continue
		}
		frameRate := parseFrameRate(stream.AvgFrameRate)
		if frameRate <= 0 {
			frameRate = parseFrameRate(stream.RFrameRate)
		}
		source.Streams = append(source.Streams, Stream{
			Index:       stream.Index,
			Type:        kind,
			Codec:       normalizeCodec(stream.CodecName),
			Language:    strings.TrimSpace(stream.Tags["language"]),
			Title:       strings.TrimSpace(stream.Tags["title"]),
			Default:     stream.Disposition != nil && stream.Disposition["default"] > 0,
			Width:       stream.Width,
			Height:      stream.Height,
			FrameRate:   frameRate,
			HDR:         detectHDR(stream.ColorTransfer, stream.ColorPrimaries, stream.PixelFormat),
			BitrateKbps: parseBitrateKbps(stream.BitRate),
		})
	}
	return source, nil
}

func parseBitrateKbps(value string) int64 {
	bits, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || bits <= 0 {
		return 0
	}
	return bits / 1000
}

func parseFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) == 1 {
		rate, _ := strconv.ParseFloat(parts[0], 64)
		return rate
	}
	num, errNum := strconv.ParseFloat(parts[0], 64)
	den, errDen := strconv.ParseFloat(parts[1], 64)
	if errNum != nil || errDen != nil || den == 0 {
		return 0
	}
	return num / den
}

func detectHDR(transfer, primaries, pixelFormat string) string {
	transfer = strings.ToLower(strings.TrimSpace(transfer))
	primaries = strings.ToLower(strings.TrimSpace(primaries))
	pixelFormat = strings.ToLower(strings.TrimSpace(pixelFormat))
	switch transfer {
	case "smpte2084":
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	// BT.2020 + 10/12-bit video without an explicit transfer value is useful
	// telemetry but is not enough to claim HDR. Keep it SDR/unknown so the
	// planner does not reject a source on an uncertain probe.
	_ = primaries
	_ = pixelFormat
	return ""
}
