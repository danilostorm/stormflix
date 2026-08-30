package transcode

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const sessionPrefix = "v5t-"

type Policy struct {
	MaxBytes        int64
	SegmentDuration time.Duration
	BatchSegments   int
	IdleTTL         time.Duration
	MinFreeBytes    int64
	MinFreePercent  int
}

func DefaultPolicy() Policy {
	return Policy{
		MaxBytes:        5 << 30,
		SegmentDuration: 4 * time.Second,
		BatchSegments:   5,
		IdleTTL:         20 * time.Minute,
		MinFreeBytes:    10 << 30,
		MinFreePercent:  5,
	}
}

type Spec struct {
	VideoStream       int
	AudioStream       int
	SourceVideoCodec  string
	TargetVideoCodec  string
	SourceAudioCodec  string
	TargetAudioCodec  string
	AudioTranscode    bool
	Width             int
	Height            int
	TargetWidth       int
	TargetHeight      int
	FrameRate         float64
	TargetFrameRate   float64
	SourceHDR         string
	ToneMap           bool
	TargetBitrateKbps int64
	DurationSeconds   float64
	Reason            string
	Quality           string
}

type EngineStatus struct {
	FFmpegPath      string   `json:"ffmpeg_path"`
	FFmpegVersion   string   `json:"ffmpeg_version"`
	HardwareAccels  []string `json:"hardware_accels"`
	VideoEncoders   []string `json:"video_encoders"`
	AudioEncoders   []string `json:"audio_encoders"`
	ToneMap         bool     `json:"tone_map"`
	ZScale          bool     `json:"zscale"`
	VAAPIDevice     string   `json:"vaapi_device,omitempty"`
	PreferredH264   string   `json:"preferred_h264"`
	PreferredHEVC   string   `json:"preferred_hevc"`
	PreferredAV1    string   `json:"preferred_av1"`
	HardwareSummary string   `json:"hardware_summary"`
}

type SessionInfo struct {
	ID                string  `json:"id"`
	UserID            int64   `json:"user_id"`
	MediaID           int64   `json:"media_id"`
	VideoCodec        string  `json:"video_codec"`
	SourceVideoCodec  string  `json:"source_video_codec"`
	AudioCodec        string  `json:"audio_codec"`
	TargetWidth       int     `json:"target_width"`
	TargetHeight      int     `json:"target_height"`
	TargetBitrateKbps int64   `json:"target_bitrate_kbps"`
	Encoder           string  `json:"encoder"`
	Hardware          string  `json:"hardware"`
	ToneMap           bool    `json:"tone_map"`
	Quality           string  `json:"quality"`
	Reason            string  `json:"reason"`
	FPS               float64 `json:"fps"`
	Speed             float64 `json:"speed"`
	CacheBytes        int64   `json:"cache_bytes"`
	Running           bool    `json:"running"`
	LastError         string  `json:"last_error,omitempty"`
	StartedAt         string  `json:"started_at"`
	LastTouch         string  `json:"last_touch"`
}

type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
	start  int
	end    int
	err    error
}

type session struct {
	mu sync.Mutex

	ID        string
	UserID    int64
	MediaID   int64
	Source    string
	Spec      Spec
	Dir       string
	StartedAt time.Time
	LastTouch time.Time
	Closed    bool
	Worker    *worker
	Encoder   string
	Hardware  string
	FPS       float64
	Speed     float64
	LastError string
}

type Manager struct {
	dir    string
	policy Policy
	engine EngineStatus

	mu       sync.Mutex
	sessions map[string]*session
}

var managers = struct {
	sync.Mutex
	items map[string]*Manager
}{items: map[string]*Manager{}}

var detected struct {
	sync.Once
	status EngineStatus
}

func SessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, sessionPrefix) {
		return raw
	}
	return sessionPrefix + raw
}

func IsSessionID(value string) bool { return strings.HasPrefix(strings.TrimSpace(value), sessionPrefix) }

func ForDataDir(dataDir string) (*Manager, error) {
	root := filepath.Join(filepath.Clean(dataDir), "transcode-cache")
	managers.Lock()
	defer managers.Unlock()
	if current := managers.items[root]; current != nil {
		return current, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
	manager := &Manager{dir: root, policy: DefaultPolicy(), engine: Detect(), sessions: map[string]*session{}}
	managers.items[root] = manager
	return manager, nil
}

func Detect() EngineStatus {
	detected.Do(func() { detected.status = detectEngine() })
	return detected.status
}

func detectEngine() EngineStatus {
	status := EngineStatus{}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return status
	}
	status.FFmpegPath = ffmpeg
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-version").Output(); err == nil {
		line := strings.SplitN(string(out), "\n", 2)[0]
		status.FFmpegVersion = strings.TrimSpace(line)
	}
	encoders := commandText(ffmpeg, "-hide_banner", "-encoders")
	hwaccels := commandText(ffmpeg, "-hide_banner", "-hwaccels")
	filters := commandText(ffmpeg, "-hide_banner", "-filters")
	status.HardwareAccels = parseSimpleLines(hwaccels, "Hardware acceleration methods:")
	for _, name := range []string{"h264_nvenc", "hevc_nvenc", "av1_nvenc", "h264_qsv", "hevc_qsv", "av1_qsv", "h264_vaapi", "hevc_vaapi", "av1_vaapi", "libx264", "libx265", "libsvtav1", "libaom-av1", "librav1e"} {
		if containsEncoder(encoders, name) {
			status.VideoEncoders = append(status.VideoEncoders, name)
		}
	}
	for _, name := range []string{"aac", "libfdk_aac", "ac3", "eac3", "libopus", "flac"} {
		if containsEncoder(encoders, name) {
			status.AudioEncoders = append(status.AudioEncoders, name)
		}
	}
	status.ToneMap = strings.Contains(filters, " tonemap ") || strings.Contains(filters, " tonemap_opencl ") || strings.Contains(filters, " tonemap_vaapi ")
	status.ZScale = strings.Contains(filters, " zscale ")
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		status.VAAPIDevice = "/dev/dri/renderD128"
	}
	status.PreferredH264 = firstAvailable(status.VideoEncoders, "h264_nvenc", "h264_qsv", "h264_vaapi", "libx264")
	status.PreferredHEVC = firstAvailable(status.VideoEncoders, "hevc_nvenc", "hevc_qsv", "hevc_vaapi", "libx265")
	status.PreferredAV1 = firstAvailable(status.VideoEncoders, "av1_nvenc", "av1_qsv", "av1_vaapi", "libsvtav1", "librav1e", "libaom-av1")
	parts := []string{}
	if strings.Contains(strings.Join(status.VideoEncoders, ","), "nvenc") {
		parts = append(parts, "NVIDIA NVENC")
	}
	if strings.Contains(strings.Join(status.VideoEncoders, ","), "qsv") {
		parts = append(parts, "Intel Quick Sync")
	}
	if status.VAAPIDevice != "" && strings.Contains(strings.Join(status.VideoEncoders, ","), "vaapi") {
		parts = append(parts, "VAAPI")
	}
	if len(parts) == 0 {
		parts = append(parts, "CPU")
	}
	status.HardwareSummary = strings.Join(parts, " + ")
	return status
}

func commandText(path string, args ...string) string {
	cmd := exec.Command(path, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func parseSimpleLines(text, after string) []string {
	out := []string{}
	started := after == ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !started {
			if strings.Contains(line, after) {
				started = true
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "ffmpeg") {
			continue
		}
		if !strings.Contains(line, " ") {
			out = append(out, line)
		}
	}
	sort.Strings(out)
	return out
}

func containsEncoder(text, name string) bool {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return true
		}
	}
	return false
}

func firstAvailable(values []string, names ...string) string {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, name := range names {
		if set[name] {
			return name
		}
	}
	return ""
}

func (m *Manager) EngineStatus() EngineStatus { return m.engine }

func validID(value string) bool {
	if !IsSessionID(value) || len(value) > 132 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func (m *Manager) Prepare(sessionID string, userID, mediaID int64, source string, spec Spec) error {
	if !validID(sessionID) || spec.VideoStream < 0 || spec.DurationSeconds <= 0 || strings.TrimSpace(spec.TargetVideoCodec) == "" {
		return errors.New("invalid transcode session")
	}
	path := filepath.Join(m.dir, sessionID)
	rel, err := filepath.Rel(m.dir, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return errors.New("transcode path escapes cache")
	}
	now := time.Now()
	m.mu.Lock()
	old := m.sessions[sessionID]
	if old != nil && old.UserID != userID {
		m.mu.Unlock()
		return errors.New("transcode session owner mismatch")
	}
	if old != nil && old.MediaID == mediaID && old.Source == source && sameSpec(old.Spec, spec) {
		old.LastTouch = now
		m.mu.Unlock()
		return nil
	}
	if old != nil {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if old != nil {
		m.stop(old)
	}
	_ = os.RemoveAll(path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	s := &session{ID: sessionID, UserID: userID, MediaID: mediaID, Source: source, Spec: spec, Dir: path, StartedAt: now, LastTouch: now}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	return nil
}

func sameSpec(a, b Spec) bool {
	return a.VideoStream == b.VideoStream && a.AudioStream == b.AudioStream && a.TargetVideoCodec == b.TargetVideoCodec && a.TargetAudioCodec == b.TargetAudioCodec && a.TargetWidth == b.TargetWidth && a.TargetHeight == b.TargetHeight && a.TargetBitrateKbps == b.TargetBitrateKbps && a.ToneMap == b.ToneMap && a.AudioTranscode == b.AudioTranscode && math.Abs(a.DurationSeconds-b.DurationSeconds) < 0.01
}

func (m *Manager) get(userID, mediaID int64, id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.UserID != userID || s.MediaID != mediaID {
		return nil, errors.New("transcode session not found")
	}
	s.LastTouch = time.Now()
	return s, nil
}

func (m *Manager) Close(userID int64, id string) bool {
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil || s.UserID != userID {
		m.mu.Unlock()
		return false
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	m.stop(s)
	_ = os.RemoveAll(s.Dir)
	return true
}

func (m *Manager) stop(s *session) {
	s.mu.Lock()
	s.Closed = true
	w := s.Worker
	s.Worker = nil
	s.mu.Unlock()
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}

func (m *Manager) Playlist(userID, mediaID int64, id string) (string, error) {
	s, err := m.get(userID, mediaID, id)
	if err != nil {
		return "", err
	}
	segmentSeconds := m.policy.SegmentDuration.Seconds()
	count := int(math.Ceil(s.Spec.DurationSeconds / segmentSeconds))
	if count < 1 {
		return "", errors.New("invalid duration")
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", int(math.Ceil(segmentSeconds)))
	for i := 0; i < count; i++ {
		if i%m.policy.BatchSegments == 0 {
			if i > 0 {
				b.WriteString("#EXT-X-DISCONTINUITY\n")
			}
			fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"/api/v1/media/%d/hls/%s/init/%d.mp4\"\n", mediaID, id, i)
		}
		d := segmentSeconds
		remaining := s.Spec.DurationSeconds - float64(i)*segmentSeconds
		if remaining < d {
			d = remaining
		}
		if d <= 0 {
			break
		}
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n/api/v1/media/%d/hls/%s/segment/%d.m4s\n", d, mediaID, id, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), nil
}

func (m *Manager) InitPath(ctx context.Context, userID, mediaID int64, id string, batch int) (string, error) {
	s, err := m.get(userID, mediaID, id)
	if err != nil {
		return "", err
	}
	if batch < 0 || batch%m.policy.BatchSegments != 0 {
		return "", errors.New("invalid transcode init batch")
	}
	path := filepath.Join(s.Dir, fmt.Sprintf("init-%06d.mp4", batch))
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		return path, nil
	}
	if err := m.ensureBatch(ctx, s, batch, batch); err != nil {
		return "", err
	}
	if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
		return "", errors.New("transcode init not generated")
	}
	return path, nil
}

func (m *Manager) SegmentPath(ctx context.Context, userID, mediaID int64, id string, segment int) (string, error) {
	s, err := m.get(userID, mediaID, id)
	if err != nil {
		return "", err
	}
	if segment < 0 || segment >= int(math.Ceil(s.Spec.DurationSeconds/m.policy.SegmentDuration.Seconds())) {
		return "", errors.New("invalid transcode segment")
	}
	path := filepath.Join(s.Dir, fmt.Sprintf("seg-%06d.m4s", segment))
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		m.trim(s, segment)
		return path, nil
	}
	batch := (segment / m.policy.BatchSegments) * m.policy.BatchSegments
	if err := m.ensureBatch(ctx, s, batch, segment); err != nil {
		return "", err
	}
	if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
		return "", errors.New("transcode segment not generated")
	}
	m.trim(s, segment)
	return path, nil
}

func (m *Manager) ensureBatch(ctx context.Context, s *session, batchStart, requested int) error {
	batchEnd := batchStart + m.policy.BatchSegments
	max := int(math.Ceil(s.Spec.DurationSeconds / m.policy.SegmentDuration.Seconds()))
	if batchEnd > max {
		batchEnd = max
	}
	expected := filepath.Join(s.Dir, fmt.Sprintf("seg-%06d.m4s", requested))
	if stat, err := os.Stat(expected); err == nil && stat.Size() > 0 {
		return nil
	}
	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		return errors.New("transcode session closed")
	}
	if w := s.Worker; w != nil && requested >= w.start && requested < w.end {
		s.mu.Unlock()
		return waitPath(ctx, expected, w)
	}
	old := s.Worker
	s.Worker = nil
	s.mu.Unlock()
	if old != nil && old.cancel != nil {
		old.cancel()
	}
	if err := m.ensureCapacity(s, float64(batchEnd-batchStart)*m.policy.SegmentDuration.Seconds()); err != nil {
		return err
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	w := &worker{cancel: cancel, done: make(chan struct{}), start: batchStart, end: batchEnd}
	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		cancel()
		return errors.New("transcode session closed")
	}
	s.Worker = w
	s.mu.Unlock()
	go m.runBatch(workerCtx, s, batchStart, batchEnd, w)
	return waitPath(ctx, expected, w)
}

func waitPath(ctx context.Context, path string, w *worker) error {
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("timed out waiting for transcoded segment")
		case <-ticker.C:
		case <-w.done:
			if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
				return nil
			}
			if w.err != nil {
				return w.err
			}
			return errors.New("transcode batch ended before segment was ready")
		}
	}
}

func (m *Manager) runBatch(ctx context.Context, s *session, batchStart, batchEnd int, w *worker) {
	defer func() {
		close(w.done)
		s.mu.Lock()
		if s.Worker == w {
			s.Worker = nil
		}
		s.mu.Unlock()
	}()
	ffmpeg := m.engine.FFmpegPath
	if ffmpeg == "" {
		w.err = errors.New("ffmpeg is not installed")
		return
	}
	candidates := m.encoderCandidates(s.Spec.TargetVideoCodec, s.Spec.ToneMap)
	if len(candidates) == 0 {
		w.err = errors.New("no compatible video encoder is available")
		return
	}
	var lastErr error
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			w.err = ctx.Err()
			return
		}
		if err := m.runCandidate(ctx, ffmpeg, s, batchStart, batchEnd, candidate); err == nil {
			s.mu.Lock()
			s.Encoder = candidate.name
			s.Hardware = candidate.hardware
			s.LastError = ""
			s.mu.Unlock()
			w.err = nil
			return
		} else {
			lastErr = err
			s.mu.Lock()
			s.LastError = err.Error()
			s.mu.Unlock()
		}
	}
	w.err = lastErr
}

type encoderCandidate struct {
	name     string
	hardware string
	vaapi    bool
}

func (m *Manager) encoderCandidates(codec string, toneMap bool) []encoderCandidate {
	codec = strings.ToLower(strings.TrimSpace(codec))
	out := []encoderCandidate{}
	add := func(name, hardware string, vaapi bool) {
		if firstAvailable(m.engine.VideoEncoders, name) != "" {
			out = append(out, encoderCandidate{name: name, hardware: hardware, vaapi: vaapi})
		}
	}
	// Tone mapping currently uses the reliable software zscale/tonemap chain.
	// Hardware tone-map surfaces differ significantly between drivers; normal
	// SDR transcodes still use hardware first with automatic CPU fallback.
	if !toneMap {
		switch codec {
		case "h264":
			add("h264_nvenc", "nvidia", false); add("h264_qsv", "qsv", false); if m.engine.VAAPIDevice != "" { add("h264_vaapi", "vaapi", true) }
		case "hevc":
			add("hevc_nvenc", "nvidia", false); add("hevc_qsv", "qsv", false); if m.engine.VAAPIDevice != "" { add("hevc_vaapi", "vaapi", true) }
		case "av1":
			add("av1_nvenc", "nvidia", false); add("av1_qsv", "qsv", false); if m.engine.VAAPIDevice != "" { add("av1_vaapi", "vaapi", true) }
		}
	}
	switch codec {
	case "h264":
		add("libx264", "cpu", false)
	case "hevc":
		add("libx265", "cpu", false)
	case "av1":
		add("libsvtav1", "cpu", false); add("librav1e", "cpu", false); add("libaom-av1", "cpu", false)
	}
	return out
}

func (m *Manager) runCandidate(ctx context.Context, ffmpeg string, s *session, batchStart, batchEnd int, candidate encoderCandidate) error {
	segmentSeconds := m.policy.SegmentDuration.Seconds()
	start := float64(batchStart) * segmentSeconds
	end := math.Min(s.Spec.DurationSeconds, float64(batchEnd)*segmentSeconds)
	duration := end - start
	playlist := filepath.Join(s.Dir, fmt.Sprintf("batch-%06d.m3u8", batchStart))
	initName := fmt.Sprintf("init-%06d.mp4", batchStart)
	segmentPattern := filepath.Join(s.Dir, "seg-%06d.m4s")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "warning", "-nostats", "-progress", "pipe:1"}
	if candidate.vaapi {
		args = append(args, "-vaapi_device", m.engine.VAAPIDevice)
	}
	args = append(args, "-ss", fmt.Sprintf("%.6f", start), "-i", s.Source, "-t", fmt.Sprintf("%.6f", duration), "-map", fmt.Sprintf("0:%d", s.Spec.VideoStream))
	if s.Spec.AudioStream >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Spec.AudioStream))
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")

	vf := m.videoFilter(s.Spec, candidate)
	if vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args, encoderArgs(candidate, s.Spec)...)
	if s.Spec.AudioStream >= 0 {
		if s.Spec.AudioTranscode || strings.EqualFold(s.Spec.TargetAudioCodec, "aac") && !strings.EqualFold(s.Spec.SourceAudioCodec, "aac") {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-b:a", "256k", "-ac", "2", "-ar", "48000")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}
	if strings.EqualFold(s.Spec.TargetVideoCodec, "hevc") {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args,
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%.3f)", segmentSeconds),
		"-avoid_negative_ts", "make_zero", "-max_muxing_queue_size", "4096",
		"-f", "hls", "-hls_time", fmt.Sprintf("%.3f", segmentSeconds), "-hls_list_size", "0", "-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", initName,
		"-hls_segment_options", "movflags=+frag_discont+skip_sidx", "-hls_flags", "independent_segments+temp_file",
		"-start_number", strconv.Itoa(batchStart), "-hls_segment_filename", segmentPattern, "-y", playlist,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			s.mu.Lock()
			switch parts[0] {
			case "fps":
				s.FPS, _ = strconv.ParseFloat(parts[1], 64)
			case "speed":
				s.Speed, _ = strconv.ParseFloat(strings.TrimSuffix(parts[1], "x"), 64)
			}
			s.mu.Unlock()
		}
	}()
	runErr := cmd.Wait()
	<-done
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 1800 {
			msg = msg[len(msg)-1800:]
		}
		if msg == "" {
			msg = runErr.Error()
		}
		return fmt.Errorf("%s transcode failed: %s", candidate.name, msg)
	}
	_ = os.Remove(playlist)
	return nil
}

func encoderArgs(candidate encoderCandidate, spec Spec) []string {
	bitrate := spec.TargetBitrateKbps
	if bitrate <= 0 {
		bitrate = 8000
	}
	b := strconv.FormatInt(bitrate, 10) + "k"
	buf := strconv.FormatInt(bitrate*2, 10) + "k"
	switch {
	case strings.HasSuffix(candidate.name, "_nvenc"):
		return []string{"-c:v", candidate.name, "-preset", "p4", "-tune", "hq", "-rc", "vbr", "-b:v", b, "-maxrate", b, "-bufsize", buf}
	case strings.HasSuffix(candidate.name, "_qsv"):
		return []string{"-c:v", candidate.name, "-preset", "veryfast", "-b:v", b, "-maxrate", b, "-bufsize", buf}
	case strings.HasSuffix(candidate.name, "_vaapi"):
		return []string{"-c:v", candidate.name, "-b:v", b, "-maxrate", b, "-bufsize", buf}
	case candidate.name == "libx264":
		return []string{"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-crf", "21", "-maxrate", b, "-bufsize", buf, "-pix_fmt", "yuv420p"}
	case candidate.name == "libx265":
		return []string{"-c:v", "libx265", "-preset", "fast", "-crf", "24", "-maxrate", b, "-bufsize", buf, "-pix_fmt", "yuv420p"}
	case candidate.name == "libsvtav1":
		return []string{"-c:v", "libsvtav1", "-preset", "8", "-crf", "30", "-b:v", b, "-pix_fmt", "yuv420p"}
	default:
		return []string{"-c:v", candidate.name, "-b:v", b, "-pix_fmt", "yuv420p"}
	}
}

func (m *Manager) videoFilter(spec Spec, candidate encoderCandidate) string {
	w, h := spec.TargetWidth, spec.TargetHeight
	if w <= 0 {
		w = spec.Width
	}
	if h <= 0 {
		h = spec.Height
	}
	filters := []string{}
	if spec.ToneMap && m.engine.ToneMap && m.engine.ZScale {
		filters = append(filters, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709", "tonemap=tonemap=hable:desat=0", "zscale=t=bt709:m=bt709:r=tv", "format=yuv420p")
	}
	if candidate.vaapi {
		filters = append(filters, "format=nv12", "hwupload")
		if w > 0 && h > 0 {
			filters = append(filters, fmt.Sprintf("scale_vaapi=w=%d:h=%d:format=nv12", w, h))
		}
	} else if w > 0 && h > 0 && (w != spec.Width || h != spec.Height) {
		filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=lanczos", w, h))
	}
	if spec.TargetFrameRate > 0 && spec.FrameRate > spec.TargetFrameRate+0.01 {
		filters = append(filters, fmt.Sprintf("fps=%.3f", spec.TargetFrameRate))
	}
	return strings.Join(filters, ",")
}

func (m *Manager) trim(s *session, current int) {
	keepBehind := m.policy.BatchSegments * 2
	cut := current - keepBehind
	if cut <= 0 {
		return
	}
	entries, _ := os.ReadDir(s.Dir)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "seg-") && strings.HasSuffix(name, ".m4s") {
			idx, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "seg-"), ".m4s"))
			if idx < cut {
				_ = os.Remove(filepath.Join(s.Dir, name))
			}
		}
		if strings.HasPrefix(name, "init-") && strings.HasSuffix(name, ".mp4") {
			idx, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "init-"), ".mp4"))
			if idx+m.policy.BatchSegments < cut {
				_ = os.Remove(filepath.Join(s.Dir, name))
			}
		}
	}
}

func (m *Manager) ensureCapacity(s *session, duration float64) error {
	m.cleanupIdle()
	usage := dirSize(m.dir)
	bitrate := s.Spec.TargetBitrateKbps
	if bitrate <= 0 {
		bitrate = 12000
	}
	estimate := int64(float64(bitrate)*1000/8*duration*1.25) + 8<<20
	if m.policy.MaxBytes > 0 && usage+estimate > m.policy.MaxBytes {
		return errors.New("transcode cache reached its global SSD budget")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.dir, &stat); err == nil {
		free := int64(stat.Bavail) * int64(stat.Bsize)
		total := int64(stat.Blocks) * int64(stat.Bsize)
		min := m.policy.MinFreeBytes
		if total > 0 && m.policy.MinFreePercent > 0 {
			pct := total * int64(m.policy.MinFreePercent) / 100
			if pct > min {
				min = pct
			}
		}
		if free-estimate < min {
			return errors.New("transcode cache refused work to preserve free SSD space")
		}
	}
	return nil
}

func (m *Manager) cleanupIdle() {
	cut := time.Now().Add(-m.policy.IdleTTL)
	m.mu.Lock()
	remove := []*session{}
	for id, s := range m.sessions {
		if s.LastTouch.Before(cut) {
			delete(m.sessions, id)
			remove = append(remove, s)
		}
	}
	m.mu.Unlock()
	for _, s := range remove {
		m.stop(s)
		_ = os.RemoveAll(s.Dir)
	}
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (m *Manager) Sessions() []SessionInfo {
	m.cleanupIdle()
	m.mu.Lock()
	items := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, s)
	}
	m.mu.Unlock()
	out := make([]SessionInfo, 0, len(items))
	for _, s := range items {
		s.mu.Lock()
		info := SessionInfo{
			ID: s.ID, UserID: s.UserID, MediaID: s.MediaID, VideoCodec: s.Spec.TargetVideoCodec, SourceVideoCodec: s.Spec.SourceVideoCodec,
			AudioCodec: s.Spec.TargetAudioCodec, TargetWidth: s.Spec.TargetWidth, TargetHeight: s.Spec.TargetHeight, TargetBitrateKbps: s.Spec.TargetBitrateKbps,
			Encoder: s.Encoder, Hardware: s.Hardware, ToneMap: s.Spec.ToneMap, Quality: s.Spec.Quality, Reason: s.Spec.Reason,
			FPS: s.FPS, Speed: s.Speed, CacheBytes: dirSize(s.Dir), Running: s.Worker != nil, LastError: s.LastError,
			StartedAt: s.StartedAt.UTC().Format(time.RFC3339), LastTouch: s.LastTouch.UTC().Format(time.RFC3339),
		}
		s.mu.Unlock()
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

func (m *Manager) CacheStatus() map[string]any {
	m.cleanupIdle()
	var stat syscall.Statfs_t
	free := int64(0)
	if syscall.Statfs(m.dir, &stat) == nil {
		free = int64(stat.Bavail) * int64(stat.Bsize)
	}
	return map[string]any{
		"directory": m.dir, "usage_bytes": dirSize(m.dir), "max_bytes": m.policy.MaxBytes, "free_bytes": free,
		"segment_seconds": m.policy.SegmentDuration.Seconds(), "batch_segments": m.policy.BatchSegments, "sessions": len(m.Sessions()),
		"min_free_bytes": m.policy.MinFreeBytes, "min_free_percent": m.policy.MinFreePercent,
	}
}
