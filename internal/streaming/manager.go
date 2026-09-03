package streaming

import (
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

	"github.com/danilostorm/stormflix/internal/transcode"
)

const sessionPrefix = "web53-"

const (
	segmentDuration = 2 * time.Second
	seekAheadWindow = 12
)

type Policy struct {
	MaxBytes            int64
	MinFreeBytes        int64
	MinFreePercent      int
	IdleTTL             time.Duration
	WorkerIdleTTL       time.Duration
	MaxAheadSegments    int
	ResumeAheadSegments int
	KeepBehindSegments  int
}

func DefaultPolicy() Policy {
	return Policy{
		MaxBytes: 5 << 30, MinFreeBytes: 10 << 30, MinFreePercent: 5,
		IdleTTL: 20 * time.Minute, WorkerIdleTTL: 90 * time.Second,
		MaxAheadSegments: 30, ResumeAheadSegments: 10, KeepBehindSegments: 120,
	}
}

func normalizePolicy(policy Policy) Policy {
	defaults := DefaultPolicy()
	if policy.MaxBytes <= 0 {
		policy.MaxBytes = defaults.MaxBytes
	}
	if policy.MinFreeBytes <= 0 {
		policy.MinFreeBytes = defaults.MinFreeBytes
	}
	if policy.MinFreePercent <= 0 {
		policy.MinFreePercent = defaults.MinFreePercent
	}
	if policy.IdleTTL <= 0 {
		policy.IdleTTL = defaults.IdleTTL
	}
	if policy.WorkerIdleTTL <= 0 {
		policy.WorkerIdleTTL = defaults.WorkerIdleTTL
	}
	if policy.MaxAheadSegments < 2 {
		policy.MaxAheadSegments = defaults.MaxAheadSegments
	}
	if policy.ResumeAheadSegments < 1 || policy.ResumeAheadSegments >= policy.MaxAheadSegments {
		policy.ResumeAheadSegments = policy.MaxAheadSegments / 3
	}
	if policy.KeepBehindSegments < 1 {
		policy.KeepBehindSegments = defaults.KeepBehindSegments
	}
	return policy
}

type Spec struct {
	VideoStream       int
	AudioStream       int
	SourceVideoCodec  string
	TargetVideoCodec  string
	SourceAudioCodec  string
	TargetAudioCodec  string
	VideoTranscode    bool
	AudioTranscode    bool
	Width             int
	Height            int
	TargetWidth       int
	TargetHeight      int
	FrameRate         float64
	TargetFrameRate   float64
	ToneMap           bool
	TargetBitrateKbps int64
	DurationSeconds   float64
	StartSeconds      float64
	Quality           string
}

type worker struct {
	cancel context.CancelFunc
	done   chan struct{}
	start  int
	err    error
}

type session struct {
	mu        sync.Mutex
	restartMu sync.Mutex

	ID               string
	UserID           int64
	MediaID          int64
	Source           string
	Spec             Spec
	Dir              string
	LastTouch        time.Time
	StartedAt        time.Time
	Closed           bool
	worker           *worker
	Encoder          string
	Hardware         string
	ProcessID        int
	WorkerPaused     bool
	PlaybackState    string
	RequestedSegment int
	ResourceWait     time.Duration
	LastError        string
}

type Manager struct {
	dir    string
	engine transcode.EngineStatus
	policy Policy

	mu       sync.Mutex
	sessions map[string]*session
}

var managers = struct {
	sync.Mutex
	items map[string]*Manager
}{items: map[string]*Manager{}}

func SessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, sessionPrefix) {
		return raw
	}
	return sessionPrefix + raw
}

func IsSessionID(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), sessionPrefix)
}

func ForDataDir(dataDir string) (*Manager, error) {
	return ForDataDirWithPolicy(dataDir, DefaultPolicy())
}

func ForDataDirWithPolicy(dataDir string, policy Policy) (*Manager, error) {
	root := filepath.Join(filepath.Clean(dataDir), "web-stream-cache")
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
	m := &Manager{dir: root, engine: transcode.Detect(), policy: normalizePolicy(policy), sessions: map[string]*session{}}
	managers.items[root] = m
	go m.cleanupLoop()
	return m, nil
}

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
	if !validID(sessionID) || spec.VideoStream < 0 || spec.DurationSeconds <= 0 {
		return errors.New("invalid web playback session")
	}
	if spec.VideoTranscode && strings.TrimSpace(spec.TargetVideoCodec) == "" {
		return errors.New("web video transcode requires a target codec")
	}
	if spec.StartSeconds < 0 {
		spec.StartSeconds = 0
	}
	if spec.StartSeconds >= spec.DurationSeconds {
		spec.StartSeconds = math.Max(0, spec.DurationSeconds-2)
	}
	path := filepath.Join(m.dir, sessionID)
	rel, err := filepath.Rel(m.dir, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("web playback path escapes cache")
	}
	now := time.Now()

	m.mu.Lock()
	old := m.sessions[sessionID]
	if old != nil && old.UserID != userID {
		m.mu.Unlock()
		return errors.New("web playback session owner mismatch")
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

	start := segmentForTime(spec.StartSeconds)
	s := &session{ID: sessionID, UserID: userID, MediaID: mediaID, Source: source, Spec: spec, Dir: path, LastTouch: now, StartedAt: now, RequestedSegment: start}
	if err := m.ensureCapacity(s); err != nil {
		_ = os.RemoveAll(path)
		return err
	}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	return m.restartAt(s, start)
}

func segmentForTime(seconds float64) int {
	if seconds <= 0 {
		return 0
	}
	return int(math.Floor(seconds / segmentDuration.Seconds()))
}

func (m *Manager) get(userID, mediaID int64, id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.UserID != userID || s.MediaID != mediaID {
		return nil, errors.New("web playback session not found")
	}
	s.LastTouch = time.Now()
	return s, nil
}

func (m *Manager) Touch(userID int64, id string) {
	if !validID(id) {
		return
	}
	m.mu.Lock()
	if s := m.sessions[id]; s != nil && s.UserID == userID {
		s.LastTouch = time.Now()
	}
	m.mu.Unlock()
}

// SetPlaybackState lets the continuous worker stop consuming CPU, disk and
// remote bandwidth while the browser is paused. Position also moves the
// requested segment forward so a stopped-ahead worker can resume promptly.
func (m *Manager) SetPlaybackState(userID int64, id, state string, positionSeconds float64) {
	if !validID(id) {
		return
	}
	state = strings.ToLower(strings.TrimSpace(state))
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil || s.UserID != userID {
		m.mu.Unlock()
		return
	}
	s.LastTouch = time.Now()
	m.mu.Unlock()
	s.mu.Lock()
	s.PlaybackState = state
	if positionSeconds >= 0 {
		s.RequestedSegment = segmentForTime(positionSeconds)
	}
	s.mu.Unlock()
}

func (m *Manager) Close(userID int64, id string) bool {
	if !validID(id) {
		return false
	}
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
	w := s.worker
	s.worker = nil
	s.mu.Unlock()
	if w != nil && w.cancel != nil {
		w.cancel()
		select {
		case <-w.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Manager) Playlist(userID, mediaID int64, id string) (string, error) {
	s, err := m.get(userID, mediaID, id)
	if err != nil {
		return "", err
	}
	count := int(math.Ceil(s.Spec.DurationSeconds / segmentDuration.Seconds()))
	if count < 1 {
		return "", errors.New("invalid web playback duration")
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", int(math.Ceil(segmentDuration.Seconds())))
	b.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")
	for i := 0; i < count; i++ {
		d := segmentDuration.Seconds()
		remaining := s.Spec.DurationSeconds - float64(i)*segmentDuration.Seconds()
		if remaining < d {
			d = remaining
		}
		if d <= 0 {
			break
		}
		fmt.Fprintf(&b, "#EXTINF:%.6f,\nseg-%06d.m4s\n", d, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), nil
}

func (m *Manager) FilePath(ctx context.Context, userID, mediaID int64, id, name string) (string, string, error) {
	s, err := m.get(userID, mediaID, id)
	if err != nil {
		return "", "", err
	}
	if name == "init.mp4" {
		path := filepath.Join(s.Dir, name)
		if err := waitForFile(ctx, path, 35*time.Second); err != nil {
			return "", "", err
		}
		return path, "video/mp4", nil
	}
	segment, ok := parseSegmentName(name)
	if !ok {
		return "", "", errors.New("invalid web playback fragment")
	}
	max := int(math.Ceil(s.Spec.DurationSeconds/segmentDuration.Seconds())) - 1
	if segment < 0 || segment > max {
		return "", "", errors.New("web playback fragment outside media duration")
	}
	s.mu.Lock()
	s.RequestedSegment = segment
	s.mu.Unlock()
	path := filepath.Join(s.Dir, name)
	if stat, statErr := os.Stat(path); statErr == nil && stat.Size() > 0 {
		m.trimBehind(s, segment)
		return path, "video/mp4", nil
	}
	if err := m.ensureAt(s, segment); err != nil {
		return "", "", err
	}
	if err := waitForFile(ctx, path, 45*time.Second); err != nil {
		return "", "", err
	}
	m.trimBehind(s, segment)
	return path, "video/mp4", nil
}

func parseSegmentName(name string) (int, bool) {
	if !strings.HasPrefix(name, "seg-") || !strings.HasSuffix(name, ".m4s") {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(name, "seg-"), ".m4s")
	if len(raw) != 6 {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 0
}

func waitForFile(ctx context.Context, path string, timeout time.Duration) error {
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("timed out waiting for web playback fragment")
		case <-ticker.C:
			if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
				return nil
			}
		}
	}
}

func (m *Manager) ensureAt(s *session, requested int) error {
	path := filepath.Join(s.Dir, fmt.Sprintf("seg-%06d.m4s", requested))
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		return nil
	}
	maxGenerated := maxGeneratedSegment(s.Dir)
	s.mu.Lock()
	w := s.worker
	closed := s.Closed
	s.mu.Unlock()
	if closed {
		return errors.New("web playback session closed")
	}
	if w == nil {
		return m.restartAt(s, requested)
	}
	if requested < w.start || requested > w.start+seekAheadWindow && maxGenerated < 0 || maxGenerated >= 0 && (requested > maxGenerated+seekAheadWindow || requested < maxGenerated-m.policy.KeepBehindSegments) {
		return m.restartAt(s, requested)
	}
	return nil
}

func maxGeneratedSegment(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return -1
	}
	max := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if n, ok := parseSegmentName(entry.Name()); ok && n > max {
			if info, infoErr := entry.Info(); infoErr == nil && info.Size() > 0 {
				max = n
			}
		}
	}
	return max
}

func (m *Manager) restartAt(s *session, start int) error {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	if start < 0 {
		start = 0
	}
	max := int(math.Ceil(s.Spec.DurationSeconds/segmentDuration.Seconds())) - 1
	if start > max {
		start = max
	}

	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		return errors.New("web playback session closed")
	}
	old := s.worker
	s.worker = nil
	s.mu.Unlock()
	if old != nil && old.cancel != nil {
		old.cancel()
		select {
		case <-old.done:
		case <-time.After(2 * time.Second):
		}
	}
	_ = os.Remove(filepath.Join(s.Dir, "init.mp4"))
	_ = os.Remove(filepath.Join(s.Dir, "worker.m3u8"))
	_ = os.Remove(filepath.Join(s.Dir, "worker.m3u8.tmp"))

	ctx, cancel := context.WithCancel(context.Background())
	w := &worker{cancel: cancel, done: make(chan struct{}), start: start}
	s.mu.Lock()
	if s.Closed {
		s.mu.Unlock()
		cancel()
		return errors.New("web playback session closed")
	}
	s.worker = w
	s.mu.Unlock()
	m.mu.Lock()
	s.LastTouch = time.Now()
	m.mu.Unlock()
	go m.run(ctx, s, w)
	return nil
}

func (m *Manager) run(ctx context.Context, s *session, w *worker) {
	defer func() {
		close(w.done)
		s.mu.Lock()
		if s.worker == w {
			s.worker = nil
		}
		s.mu.Unlock()
	}()
	ffmpeg := m.engine.FFmpegPath
	if ffmpeg == "" {
		w.err = errors.New("ffmpeg is not installed")
		return
	}
	candidates := m.encoderCandidates(s.Spec)
	if len(candidates) == 0 {
		w.err = errors.New("no compatible web playback encoder is available")
		return
	}
	var lastErr error
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return
		}
		m.removeGeneratedFrom(s, w.start)
		if err := m.runCandidate(ctx, ffmpeg, s, w.start, candidate); err == nil {
			s.mu.Lock()
			s.Encoder = candidate.name
			s.Hardware = candidate.hardware
			s.LastError = ""
			s.mu.Unlock()
			w.err = nil
			return
		} else {
			if ctx.Err() != nil {
				return
			}
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
	copy     bool
}

func (m *Manager) encoderCandidates(spec Spec) []encoderCandidate {
	if !spec.VideoTranscode {
		return []encoderCandidate{{name: "copy", hardware: "stream-copy", copy: true}}
	}
	codec := strings.ToLower(strings.TrimSpace(spec.TargetVideoCodec))
	available := map[string]bool{}
	for _, value := range m.engine.VideoEncoders {
		available[value] = true
	}
	out := []encoderCandidate{}
	add := func(name, hardware string, vaapi bool) {
		if available[name] {
			out = append(out, encoderCandidate{name: name, hardware: hardware, vaapi: vaapi})
		}
	}
	// Software HDR tone mapping can feed NVENC ordinary yuv420p frames safely,
	// offloading encode even when GPU-specific tone-map filters are unavailable.
	switch codec {
	case "h264":
		add("h264_nvenc", "nvidia", false)
		if !spec.ToneMap {
			add("h264_qsv", "qsv", false)
			if m.engine.VAAPIDevice != "" {
				add("h264_vaapi", "vaapi", true)
			}
		}
	case "hevc", "h265":
		add("hevc_nvenc", "nvidia", false)
		if !spec.ToneMap {
			add("hevc_qsv", "qsv", false)
			if m.engine.VAAPIDevice != "" {
				add("hevc_vaapi", "vaapi", true)
			}
		}
	case "av1":
		add("av1_nvenc", "nvidia", false)
		if !spec.ToneMap {
			add("av1_qsv", "qsv", false)
			if m.engine.VAAPIDevice != "" {
				add("av1_vaapi", "vaapi", true)
			}
		}
	}
	switch codec {
	case "h264":
		add("libx264", "cpu", false)
	case "hevc", "h265":
		add("libx265", "cpu", false)
	case "av1":
		add("libsvtav1", "cpu", false)
		add("librav1e", "cpu", false)
		add("libaom-av1", "cpu", false)
	}
	return out
}

func (m *Manager) runCandidate(ctx context.Context, ffmpeg string, s *session, startSegment int, candidate encoderCandidate) error {
	release, waited, err := transcode.AcquireProcess(ctx, !candidate.copy)
	if err != nil {
		return fmt.Errorf("wait for playback process capacity: %w", err)
	}
	defer release()
	s.mu.Lock()
	s.ResourceWait += waited
	s.mu.Unlock()

	startSeconds := float64(startSegment) * segmentDuration.Seconds()
	remaining := s.Spec.DurationSeconds - startSeconds
	if remaining <= 0 {
		return errors.New("web playback start is outside media duration")
	}
	playlist := filepath.Join(s.Dir, "worker.m3u8")
	segmentPattern := filepath.Join(s.Dir, "seg-%06d.m4s")
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "warning"}
	if !candidate.copy {
		threads := strconv.Itoa(transcode.CPUThreadLimit())
		args = append(args, "-filter_threads", threads, "-filter_complex_threads", threads, "-threads", threads)
	}
	if candidate.vaapi {
		args = append(args, "-vaapi_device", m.engine.VAAPIDevice)
	}
	if startSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.6f", startSeconds))
	}
	// Keep a small lead over realtime instead of racing through the whole movie.
	// This mirrors the throttled-session idea used by mature media servers while
	// still producing the first 2-second fragment quickly.
	args = append(args, "-readrate", "1.15", "-i", s.Source, "-t", fmt.Sprintf("%.6f", remaining), "-map", fmt.Sprintf("0:%d", s.Spec.VideoStream))
	if s.Spec.AudioStream >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Spec.AudioStream))
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")

	if candidate.copy {
		args = append(args, "-c:v", "copy")
	} else {
		vf := m.videoFilter(s.Spec, candidate)
		if vf != "" {
			args = append(args, "-vf", vf)
		}
		args = append(args, encoderArgs(candidate, s.Spec)...)
	}
	if s.Spec.AudioStream >= 0 {
		if s.Spec.AudioTranscode || strings.EqualFold(s.Spec.TargetAudioCodec, "aac") && !strings.EqualFold(s.Spec.SourceAudioCodec, "aac") {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-b:a", "256k", "-ac", "2", "-ar", "48000")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}
	if strings.EqualFold(s.Spec.TargetVideoCodec, "hevc") || strings.EqualFold(s.Spec.TargetVideoCodec, "h265") || candidate.copy && (strings.EqualFold(s.Spec.SourceVideoCodec, "hevc") || strings.EqualFold(s.Spec.SourceVideoCodec, "h265")) {
		args = append(args, "-tag:v", "hvc1")
	}
	if !candidate.copy {
		args = append(args, "-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%.3f)", segmentDuration.Seconds()))
	}
	flags := "independent_segments+temp_file"
	if candidate.copy {
		// Stream copy cannot manufacture keyframes. split_by_time keeps the global
		// 2-second timeline stable while the single FFmpeg process preserves GOP
		// continuity across adjacent fragments.
		flags = "split_by_time+temp_file"
	}
	args = append(args,
		"-avoid_negative_ts", "make_zero",
		"-max_muxing_queue_size", "4096",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%.3f", segmentDuration.Seconds()),
		"-hls_list_size", "0",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", flags,
		"-start_number", strconv.Itoa(startSegment),
		"-hls_segment_filename", segmentPattern,
		"-y", playlist,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	s.ProcessID = cmd.Process.Pid
	s.WorkerPaused = false
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.ProcessID == cmd.Process.Pid {
			s.ProcessID = 0
			s.WorkerPaused = false
		}
		s.mu.Unlock()
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	paused := false
	pressure := false
	lastPressureCheck := time.Time{}
	resume := func() {
		if paused && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGCONT)
			paused = false
			s.mu.Lock()
			s.WorkerPaused = false
			s.mu.Unlock()
		}
	}
	stopAhead := func() {
		if !paused && cmd.Process != nil {
			if cmd.Process.Signal(syscall.SIGSTOP) == nil {
				paused = true
				s.mu.Lock()
				s.WorkerPaused = true
				s.mu.Unlock()
			}
		}
	}

	var runErr error
	for runErr == nil {
		select {
		case runErr = <-done:
			if runErr == nil {
				return nil
			}
		case <-ctx.Done():
			resume()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
			return ctx.Err()
		case <-ticker.C:
			s.mu.Lock()
			requested := s.RequestedSegment
			state := s.PlaybackState
			closed := s.Closed
			s.mu.Unlock()
			m.mu.Lock()
			lastTouch := s.LastTouch
			m.mu.Unlock()
			if closed || time.Since(lastTouch) > m.policy.WorkerIdleTTL {
				resume()
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-done
				return errors.New("web playback worker stopped while client was idle")
			}
			generated := maxGeneratedSegment(s.Dir)
			ahead := generated - requested
			if time.Since(lastPressureCheck) >= 5*time.Second {
				pressure = !m.withinCapacity(0)
				lastPressureCheck = time.Now()
			}
			if state == "paused" || ahead >= m.policy.MaxAheadSegments || pressure {
				stopAhead()
			} else if state != "paused" && ahead <= m.policy.ResumeAheadSegments && !pressure {
				resume()
			}
		}
	}
	resume()
	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 1800 {
			msg = msg[len(msg)-1800:]
		}
		if msg == "" {
			msg = runErr.Error()
		}
		return fmt.Errorf("%s web playback failed: %s", candidate.name, msg)
	}
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
		return []string{"-c:v", "libx264", "-threads", strconv.Itoa(transcode.CPUThreadLimit()), "-preset", "superfast", "-profile:v", "high", "-crf", "21", "-maxrate", b, "-bufsize", buf, "-pix_fmt", "yuv420p"}
	case candidate.name == "libx265":
		return []string{"-c:v", "libx265", "-threads", strconv.Itoa(transcode.CPUThreadLimit()), "-x265-params", "pools=" + strconv.Itoa(transcode.CPUThreadLimit()), "-preset", "veryfast", "-crf", "24", "-maxrate", b, "-bufsize", buf, "-pix_fmt", "yuv420p"}
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
		// Live compatibility transcodes favor low CPU latency over offline-quality
		// resampling. Direct Play is unaffected, and 4K->1080p gets the cheapest
		// scaler because its source contains ample spatial detail.
		flags := "bicubic"
		if spec.Height >= 2000 && h <= 1080 {
			flags = "fast_bilinear"
		}
		filters = append(filters, fmt.Sprintf("scale=%d:%d:flags=%s", w, h, flags))
	}
	if spec.TargetFrameRate > 0 && spec.FrameRate > spec.TargetFrameRate+0.01 {
		filters = append(filters, fmt.Sprintf("fps=%.3f", spec.TargetFrameRate))
	}
	return strings.Join(filters, ",")
}

func (m *Manager) removeGeneratedFrom(s *session, start int) {
	entries, _ := os.ReadDir(s.Dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if n, ok := parseSegmentName(entry.Name()); ok && n >= start {
			_ = os.Remove(filepath.Join(s.Dir, entry.Name()))
		}
	}
	_ = os.Remove(filepath.Join(s.Dir, "init.mp4"))
	_ = os.Remove(filepath.Join(s.Dir, "worker.m3u8"))
}

func (m *Manager) trimBehind(s *session, current int) {
	cut := current - m.policy.KeepBehindSegments
	if cut <= 0 {
		return
	}
	entries, _ := os.ReadDir(s.Dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if n, ok := parseSegmentName(entry.Name()); ok && n < cut {
			_ = os.Remove(filepath.Join(s.Dir, entry.Name()))
		}
	}
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-m.policy.IdleTTL)
		m.mu.Lock()
		stale := make([]*session, 0)
		for id, s := range m.sessions {
			if s.LastTouch.Before(cutoff) {
				delete(m.sessions, id)
				stale = append(stale, s)
			}
		}
		m.mu.Unlock()
		for _, s := range stale {
			m.stop(s)
			_ = os.RemoveAll(s.Dir)
		}
	}
}

func (m *Manager) ensureCapacity(s *session) error {
	m.cleanupIdle()
	bitrate := s.Spec.TargetBitrateKbps
	if bitrate <= 0 {
		bitrate = 12000
	}
	windowSeconds := float64(m.policy.KeepBehindSegments+m.policy.MaxAheadSegments) * segmentDuration.Seconds()
	estimate := int64(float64(bitrate)*1000/8*windowSeconds*1.25) + 8<<20
	if estimate > m.policy.MaxBytes/2 {
		estimate = m.policy.MaxBytes / 2
	}
	if !m.withinCapacity(estimate) {
		return errors.New("web stream cache refused work to preserve its disk budget")
	}
	return nil
}

func (m *Manager) withinCapacity(extra int64) bool {
	if m.policy.MaxBytes > 0 && directorySize(m.dir)+extra > m.policy.MaxBytes {
		return false
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.dir, &stat); err != nil {
		return true
	}
	free := int64(stat.Bavail) * int64(stat.Bsize)
	total := int64(stat.Blocks) * int64(stat.Bsize)
	reserve := m.policy.MinFreeBytes
	if total > 0 && m.policy.MinFreePercent > 0 {
		percent := total * int64(m.policy.MinFreePercent) / 100
		if percent > reserve {
			reserve = percent
		}
	}
	return free-extra >= reserve
}

func (m *Manager) cleanupIdle() {
	cutoff := time.Now().Add(-m.policy.IdleTTL)
	m.mu.Lock()
	stale := make([]*session, 0)
	for id, s := range m.sessions {
		if s.LastTouch.Before(cutoff) {
			delete(m.sessions, id)
			stale = append(stale, s)
		}
	}
	m.mu.Unlock()
	for _, s := range stale {
		m.stop(s)
		_ = os.RemoveAll(s.Dir)
	}
}

func directorySize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

type SessionInfo struct {
	ID               string `json:"id"`
	UserID           int64  `json:"user_id"`
	MediaID          int64  `json:"media_id"`
	Route            string `json:"route"`
	Encoder          string `json:"encoder"`
	Hardware         string `json:"hardware"`
	ProcessID        int    `json:"process_id,omitempty"`
	WorkerPaused     bool   `json:"worker_paused"`
	PlaybackState    string `json:"playback_state"`
	RequestedSegment int    `json:"requested_segment"`
	AheadSegments    int    `json:"ahead_segments"`
	CacheBytes       int64  `json:"cache_bytes"`
	ResourceWaitMS   int64  `json:"resource_wait_ms"`
	StartedAt        string `json:"started_at"`
	LastTouch        string `json:"last_touch"`
	LastError        string `json:"last_error,omitempty"`
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
		m.mu.Lock()
		lastTouch := s.LastTouch
		m.mu.Unlock()
		s.mu.Lock()
		route := "server-remux"
		if s.Spec.VideoTranscode {
			route = "server-video"
		} else if s.Spec.AudioTranscode {
			route = "server-audio"
		}
		requested := s.RequestedSegment
		info := SessionInfo{
			ID: s.ID, UserID: s.UserID, MediaID: s.MediaID, Route: route,
			Encoder: s.Encoder, Hardware: s.Hardware, ProcessID: s.ProcessID,
			WorkerPaused: s.WorkerPaused, PlaybackState: s.PlaybackState,
			RequestedSegment: requested,
			ResourceWaitMS:   s.ResourceWait.Milliseconds(), StartedAt: s.StartedAt.UTC().Format(time.RFC3339),
			LastTouch: lastTouch.UTC().Format(time.RFC3339), LastError: s.LastError,
		}
		s.mu.Unlock()
		info.CacheBytes = directorySize(s.Dir)
		generated := maxGeneratedSegment(s.Dir)
		if generated >= requested {
			info.AheadSegments = generated - requested
		}
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
		"directory": m.dir, "usage_bytes": directorySize(m.dir), "max_bytes": m.policy.MaxBytes,
		"free_bytes": free, "min_free_bytes": m.policy.MinFreeBytes, "min_free_percent": m.policy.MinFreePercent,
		"max_ahead_seconds":   m.policy.MaxAheadSegments * int(segmentDuration/time.Second),
		"keep_behind_seconds": m.policy.KeepBehindSegments * int(segmentDuration/time.Second),
		"worker_idle_seconds": int(m.policy.WorkerIdleTTL.Seconds()), "sessions": len(m.Sessions()),
	}
}
