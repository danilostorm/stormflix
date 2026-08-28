package webcompat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var materializeLocks sync.Map

func verifyMaterialized(path string, plan Plan) error {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return errors.New("ffprobe is not installed")
	}
	cmd := exec.Command(ffprobe,
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name,duration,nb_frames",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("verify compatibility file: %w", err)
	}
	var payload struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Duration  string `json:"duration"`
			Frames    string `json:"nb_frames"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return fmt.Errorf("decode compatibility verification: %w", err)
	}
	hasVideo := false
	hasAudio := false
	hasAAC := false
	audioHasTimeline := false
	for _, stream := range payload.Streams {
		switch strings.ToLower(stream.CodecType) {
		case "video":
			hasVideo = true
		case "audio":
			hasAudio = true
			if strings.EqualFold(stream.CodecName, "aac") {
				hasAAC = true
			}
			duration, _ := strconv.ParseFloat(strings.TrimSpace(stream.Duration), 64)
			framesText := strings.TrimSpace(stream.Frames)
			frames, framesErr := strconv.ParseInt(framesText, 10, 64)
			if duration > 0.5 && (framesErr != nil || frames > 0) {
				audioHasTimeline = true
			}
		}
	}
	if !hasVideo {
		return errors.New("compatibility file has no video stream")
	}
	if plan.AudioStream >= 0 && !hasAudio {
		return errors.New("compatibility file has no audio stream")
	}
	if plan.AudioTranscode && !hasAAC {
		return errors.New("compatibility file audio was not encoded as AAC")
	}
	if plan.AudioStream >= 0 && !audioHasTimeline {
		return errors.New("compatibility file audio has no readable timeline")
	}
	return nil
}

// MaterializedPath returns the deterministic cache path for a compatibility
// artifact. Keeping path calculation in one place lets the cache manager mark
// the exact final file active before ffmpeg starts writing its temporary file.
func MaterializedPath(cacheDir, cacheKey string) string {
	sum := sha256.Sum256([]byte("seekable-aac-v3|" + cacheKey))
	name := hex.EncodeToString(sum[:16]) + ".mp4"
	return filepath.Join(cacheDir, name)
}

// MaterializeSeekable creates a normal MP4 file for compatibility playback.
// The video stream is always copied. Only audio is encoded when Plan asks for
// it. A real file lets http.ServeContent provide Content-Length, Range/206 and
// a seekable timeline, unlike the old fragmented MP4 stdout pipe.
func MaterializeSeekable(ctx context.Context, source string, plan Plan, cacheDir, cacheKey string) (string, error) {
	if !plan.Available || plan.VideoStream < 0 {
		return "", errors.New("compatibility media is not available")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", errors.New("ffmpeg is not installed")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create compatibility cache: %w", err)
	}

	// v3 deliberately invalidates v2 files produced with
	// aresample=async=1:first_pts=0. On sources with shifted timestamps that
	// filter can pad a very long silent prefix even though the AAC track exists.
	finalPath := MaterializedPath(cacheDir, cacheKey)
	name := filepath.Base(finalPath)
	lockValue, _ := materializeLocks.LoadOrStore(finalPath, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if stat, statErr := os.Stat(finalPath); statErr == nil && stat.Size() > 4096 {
		if verifyMaterialized(finalPath, plan) == nil {
			return finalPath, nil
		}
		_ = os.Remove(finalPath)
	}

	tmp, err := os.CreateTemp(cacheDir, name+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("create compatibility temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-fflags", "+genpts",
		"-i", source,
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
			"-profile:a", "aac_low",
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
		"-max_muxing_queue_size", "4096",
		"-movflags", "+faststart",
		"-avoid_negative_ts", "make_zero",
		"-f", "mp4",
		"-y", tmpPath,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		msg := strings.TrimSpace(string(output))
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
		}
		return "", fmt.Errorf("ffmpeg compatibility file failed: %s", msg)
	}
	stat, err := os.Stat(tmpPath)
	if err != nil || stat.Size() <= 4096 {
		return "", errors.New("ffmpeg produced an empty compatibility file")
	}
	if err := verifyMaterialized(tmpPath, plan); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("publish compatibility file: %w", err)
	}
	return finalPath, nil
}
