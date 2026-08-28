package webcompat

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HLSPolicy controls the temporary browser streaming cache. Unlike the legacy
// compatibility MP4 cache, HLS data is session-scoped and intentionally
// disposable. It is never a permanent copy of the source media.
type HLSPolicy struct {
	MaxBytes          int64
	SegmentDuration   time.Duration
	BatchSegments     int
	IdleTTL           time.Duration
	CleanupInterval   time.Duration
	MinFreeBytes      int64
	MinFreePercent    int
	EvictionTargetPct int
}

func DefaultHLSPolicy() HLSPolicy {
	return HLSPolicy{
		MaxBytes:          5 << 30,
		SegmentDuration:   6 * time.Second,
		BatchSegments:     4,
		IdleTTL:           30 * time.Minute,
		CleanupInterval:   time.Minute,
		MinFreeBytes:      10 << 30,
		MinFreePercent:    5,
		EvictionTargetPct: 80,
	}
}

// HLSSpec is the exact stream-copy/audio policy chosen by PlaybackPlan.
type HLSSpec struct {
	VideoStream       int
	AudioStream       int
	VideoCodec        string
	AudioCodec        string
	SourceAudioCodec  string
	AudioTranscode    bool
	DurationSeconds   float64
	SourceBitrateKbps int64
}

// NeedsHLSAAC reports whether the selected audio should be encoded to AAC for
// fragmented-MP4 HLS. Video is never transcoded here.
func NeedsHLSAAC(codec string, requested bool) bool {
	if requested {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "", "aac", "ac3", "eac3", "mp3":
		return false
	default:
		return true
	}
}

type HLSStatus struct {
	Directory      string `json:"directory"`
	UsageBytes     int64  `json:"usage_bytes"`
	MaxBytes       int64  `json:"max_bytes"`
	Sessions       int    `json:"sessions"`
	Workers        int    `json:"workers"`
	Files          int    `json:"files"`
	FreeBytes      int64  `json:"free_bytes"`
	MinFreeBytes   int64  `json:"min_free_bytes"`
	MinFreePercent int    `json:"min_free_percent"`
	SegmentSeconds int    `json:"segment_seconds"`
	BatchSegments  int    `json:"batch_segments"`
	IdleTTLSeconds int64  `json:"idle_ttl_seconds"`
}

type hlsWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
	start  int
	end    int
	err    error
}

type hlsSession struct {
	mu sync.Mutex

	ID        string
	UserID    int64
	MediaID   int64
	Source    string
	Spec      HLSSpec
	Dir       string
	LastTouch time.Time
	Closed    bool
	worker    *hlsWorker
}

type HLSManager struct {
	dir string

	mu       sync.Mutex
	policy   HLSPolicy
	sessions map[string]*hlsSession
	started  bool
}

func NewHLSManager(dir string, policy HLSPolicy) (*HLSManager, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("hls cache directory is required")
	}
	policy = normalizeHLSPolicy(policy)
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create hls cache directory: %w", err)
	}

	// HLS artifacts are deliberately process/session scoped. Nothing from a
	// previous server process is reusable safely, so clear only this dedicated
	// directory at startup instead of adopting old session fragments.
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := os.RemoveAll(filepath.Join(dir, item.Name())); err != nil {
			return nil, fmt.Errorf("clear stale hls cache: %w", err)
		}
	}

	return &HLSManager{dir: dir, policy: policy, sessions: map[string]*hlsSession{}}, nil
}

func normalizeHLSPolicy(policy HLSPolicy) HLSPolicy {
	defaults := DefaultHLSPolicy()
	if policy.MaxBytes < 0 {
		policy.MaxBytes = 0
	}
	if policy.SegmentDuration < 2*time.Second || policy.SegmentDuration > 30*time.Second {
		policy.SegmentDuration = defaults.SegmentDuration
	}
	if policy.BatchSegments < 1 || policy.BatchSegments > 12 {
		policy.BatchSegments = defaults.BatchSegments
	}
	if policy.IdleTTL <= 0 {
		policy.IdleTTL = defaults.IdleTTL
	}
	if policy.CleanupInterval <= 0 {
		policy.CleanupInterval = defaults.CleanupInterval
	}
	if policy.MinFreeBytes < 0 {
		policy.MinFreeBytes = 0
	}
	if policy.MinFreePercent < 0 {
		policy.MinFreePercent = 0
	}
	if policy.MinFreePercent > 95 {
		policy.MinFreePercent = 95
	}
	if policy.EvictionTargetPct <= 0 || policy.EvictionTargetPct >= 100 {
		policy.EvictionTargetPct = defaults.EvictionTargetPct
	}
	return policy
}

func (m *HLSManager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	interval := m.policy.CleanupInterval
	m.mu.Unlock()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				m.CloseAll()
				return
			case <-ticker.C:
				if err := m.Cleanup(context.Background()); err != nil {
					log.Printf("stormflix hls cache cleanup: %v", err)
				}
			}
		}
	}()
}

func (m *HLSManager) Directory() string { return m.dir }

func validHLSSessionID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
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

func (m *HLSManager) sessionPath(id string) (string, error) {
	if !validHLSSessionID(id) {
		return "", errors.New("invalid hls playback session")
	}
	path := filepath.Join(m.dir, id)
	rel, err := filepath.Rel(m.dir, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("hls session path escapes cache directory")
	}
	return path, nil
}

func sameHLSSpec(a, b HLSSpec) bool {
	return a.VideoStream == b.VideoStream &&
		a.AudioStream == b.AudioStream &&
		strings.EqualFold(a.VideoCodec, b.VideoCodec) &&
		strings.EqualFold(a.AudioCodec, b.AudioCodec) &&
		a.AudioTranscode == b.AudioTranscode &&
		math.Abs(a.DurationSeconds-b.DurationSeconds) < 0.01 &&
		a.SourceBitrateKbps == b.SourceBitrateKbps
}

// PrepareSession registers the temporary HLS workspace. Reusing a playback
// session for a different physical source/version immediately removes the old
// fragments before the new source can generate any data.
func (m *HLSManager) PrepareSession(sessionID string, userID, mediaID int64, source string, spec HLSSpec) error {
	if spec.VideoStream < 0 || spec.DurationSeconds <= 0 {
		return errors.New("invalid hls stream specification")
	}
	path, err := m.sessionPath(sessionID)
	if err != nil {
		return err
	}
	now := time.Now()

	m.mu.Lock()
	current := m.sessions[sessionID]
	if current != nil && current.UserID != userID {
		m.mu.Unlock()
		return errors.New("hls playback session owner mismatch")
	}
	if current != nil && current.MediaID == mediaID && current.Source == source && sameHLSSpec(current.Spec, spec) {
		current.LastTouch = now
		m.mu.Unlock()
		return nil
	}
	if current != nil {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if current != nil {
		m.stopSession(current)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	session := &hlsSession{ID: sessionID, UserID: userID, MediaID: mediaID, Source: source, Spec: spec, Dir: path, LastTouch: now}
	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()
	return nil
}

func (m *HLSManager) getSession(userID, mediaID int64, sessionID string) (*hlsSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[sessionID]
	if session == nil || session.UserID != userID || session.MediaID != mediaID {
		return nil, errors.New("hls playback session not found")
	}
	session.LastTouch = time.Now()
	return session, nil
}

func (m *HLSManager) TouchSession(userID int64, sessionID string) {
	if !validHLSSessionID(sessionID) {
		return
	}
	m.mu.Lock()
	if session := m.sessions[sessionID]; session != nil && session.UserID == userID {
		session.LastTouch = time.Now()
	}
	m.mu.Unlock()
}

// CloseSession is the normal lifecycle path. The user closes/finishes a movie,
// the FFmpeg batch is cancelled and the entire session directory is removed at
// once instead of waiting for TTL/LRU cleanup.
func (m *HLSManager) CloseSession(userID int64, sessionID string) bool {
	if !validHLSSessionID(sessionID) {
		return false
	}
	m.mu.Lock()
	session := m.sessions[sessionID]
	if session == nil || session.UserID != userID {
		m.mu.Unlock()
		return false
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	m.stopSession(session)
	_ = os.RemoveAll(session.Dir)
	return true
}

func (m *HLSManager) stopSession(session *hlsSession) {
	session.mu.Lock()
	session.Closed = true
	worker := session.worker
	session.worker = nil
	session.mu.Unlock()
	if worker != nil && worker.cancel != nil {
		worker.cancel()
	}
}

func (m *HLSManager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*hlsSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = map[string]*hlsSession{}
	m.mu.Unlock()
	for _, session := range sessions {
		m.stopSession(session)
		_ = os.RemoveAll(session.Dir)
	}
}

func (m *HLSManager) Playlist(userID, mediaID int64, sessionID string) (string, error) {
	session, err := m.getSession(userID, mediaID, sessionID)
	if err != nil {
		return "", err
	}
	segmentSeconds := m.policy.SegmentDuration.Seconds()
	count := int(math.Ceil(session.Spec.DurationSeconds / segmentSeconds))
	if count < 1 {
		return "", errors.New("invalid hls media duration")
	}
	target := int(math.Ceil(segmentSeconds))
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", target)
	for i := 0; i < count; i++ {
		if i%m.policy.BatchSegments == 0 {
			if i > 0 {
				b.WriteString("#EXT-X-DISCONTINUITY\n")
			}
			fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"/api/v1/media/%d/hls/%s/init/%d.mp4\"\n", mediaID, sessionID, i)
		}
		duration := segmentSeconds
		remaining := session.Spec.DurationSeconds - float64(i)*segmentSeconds
		if remaining < duration {
			duration = remaining
		}
		if duration <= 0 {
			break
		}
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n/api/v1/media/%d/hls/%s/segment/%d.m4s\n", duration, mediaID, sessionID, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), nil
}

func (m *HLSManager) InitPath(ctx context.Context, userID, mediaID int64, sessionID string, batchStart int) (string, error) {
	if batchStart < 0 || batchStart%m.policy.BatchSegments != 0 {
		return "", errors.New("invalid hls init batch")
	}
	session, err := m.getSession(userID, mediaID, sessionID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(session.Dir, fmt.Sprintf("init-%06d.mp4", batchStart))
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		_ = os.Chtimes(path, time.Now(), time.Now())
		return path, nil
	}
	if err := m.ensureBatch(ctx, session, batchStart, batchStart); err != nil {
		return "", err
	}
	if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
		return "", errors.New("hls init segment was not generated")
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	return path, nil
}

func (m *HLSManager) SegmentPath(ctx context.Context, userID, mediaID int64, sessionID string, segment int) (string, error) {
	if segment < 0 {
		return "", errors.New("invalid hls segment")
	}
	session, err := m.getSession(userID, mediaID, sessionID)
	if err != nil {
		return "", err
	}
	maxSegment := int(math.Ceil(session.Spec.DurationSeconds/m.policy.SegmentDuration.Seconds())) - 1
	if segment > maxSegment {
		return "", errors.New("hls segment is outside media duration")
	}
	path := filepath.Join(session.Dir, fmt.Sprintf("seg-%06d.m4s", segment))
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		_ = os.Chtimes(path, time.Now(), time.Now())
		m.trimSession(session, segment)
		return path, nil
	}
	batchStart := (segment / m.policy.BatchSegments) * m.policy.BatchSegments
	if err := m.ensureBatch(ctx, session, batchStart, segment); err != nil {
		return "", err
	}
	if stat, err := os.Stat(path); err != nil || stat.Size() == 0 {
		return "", errors.New("hls media segment was not generated")
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	m.trimSession(session, segment)
	return path, nil
}

func (m *HLSManager) ensureBatch(ctx context.Context, session *hlsSession, batchStart, requestedSegment int) error {
	batchEnd := batchStart + m.policy.BatchSegments
	maxSegments := int(math.Ceil(session.Spec.DurationSeconds / m.policy.SegmentDuration.Seconds()))
	if batchEnd > maxSegments {
		batchEnd = maxSegments
	}
	if requestedSegment < batchStart || requestedSegment >= batchEnd {
		return errors.New("requested segment is outside hls batch")
	}

	expected := filepath.Join(session.Dir, fmt.Sprintf("seg-%06d.m4s", requestedSegment))
	if stat, err := os.Stat(expected); err == nil && stat.Size() > 0 {
		return nil
	}

	// Inspect/cancel a previous worker without holding the session lock across
	// global capacity cleanup. This lock ordering keeps concurrent segment
	// requests and global eviction race-free.
	session.mu.Lock()
	if session.Closed {
		session.mu.Unlock()
		return errors.New("hls playback session is closed")
	}
	if worker := session.worker; worker != nil && requestedSegment >= worker.start && requestedSegment < worker.end {
		session.mu.Unlock()
		return waitForHLSPath(ctx, expected, worker)
	}
	oldWorker := session.worker
	if oldWorker != nil {
		session.worker = nil
	}
	session.mu.Unlock()
	if oldWorker != nil && oldWorker.cancel != nil {
		oldWorker.cancel()
	}

	startIndex := batchStart
	startSeconds := float64(startIndex) * m.policy.SegmentDuration.Seconds()
	endSeconds := math.Min(session.Spec.DurationSeconds, float64(batchEnd)*m.policy.SegmentDuration.Seconds())
	batchDuration := endSeconds - startSeconds
	if batchDuration <= 0 {
		return errors.New("invalid hls batch duration")
	}
	if err := m.ensureCapacity(session, batchDuration); err != nil {
		return err
	}

	// Another request can win the worker race while capacity was being checked.
	session.mu.Lock()
	if session.Closed {
		session.mu.Unlock()
		return errors.New("hls playback session is closed")
	}
	if stat, err := os.Stat(expected); err == nil && stat.Size() > 0 {
		session.mu.Unlock()
		return nil
	}
	if worker := session.worker; worker != nil && requestedSegment >= worker.start && requestedSegment < worker.end {
		session.mu.Unlock()
		return waitForHLSPath(ctx, expected, worker)
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	worker := &hlsWorker{cancel: cancel, done: make(chan struct{}), start: startIndex, end: batchEnd}
	session.worker = worker
	session.mu.Unlock()

	go m.runBatch(workerCtx, session, batchStart, startIndex, batchEnd, worker)
	return waitForHLSPath(ctx, expected, worker)
}

func waitForHLSPath(ctx context.Context, path string, worker *hlsWorker) error {
	timer := time.NewTimer(35 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	for {
		if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("timed out waiting for hls segment")
		case <-ticker.C:
		case <-worker.done:
			if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
				return nil
			}
			if worker.err != nil {
				return worker.err
			}
			return errors.New("hls batch ended before requested segment was ready")
		}
	}
}

func (m *HLSManager) runBatch(ctx context.Context, session *hlsSession, batchStart, startIndex, batchEnd int, worker *hlsWorker) {
	var resultErr error
	defer func() {
		worker.err = resultErr
		close(worker.done)
		session.mu.Lock()
		if session.worker == worker {
			session.worker = nil
		}
		session.mu.Unlock()
	}()

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		resultErr = errors.New("ffmpeg is not installed")
		return
	}

	segmentSeconds := m.policy.SegmentDuration.Seconds()
	startSeconds := float64(startIndex) * segmentSeconds
	endSeconds := math.Min(session.Spec.DurationSeconds, float64(batchEnd)*segmentSeconds)
	duration := endSeconds - startSeconds
	playlistPath := filepath.Join(session.Dir, fmt.Sprintf("batch-%06d.m3u8", batchStart))
	initName := fmt.Sprintf("init-%06d.mp4", batchStart)
	segmentPattern := filepath.Join(session.Dir, "seg-%06d.m4s")

	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.6f", startSeconds),
		"-i", session.Source,
		"-t", fmt.Sprintf("%.6f", duration),
		"-map", fmt.Sprintf("0:%d", session.Spec.VideoStream),
	}
	if session.Spec.AudioStream >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:%d", session.Spec.AudioStream))
	}
	args = append(args, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1", "-c:v", "copy")
	if session.Spec.AudioStream >= 0 {
		if NeedsHLSAAC(session.Spec.AudioCodec, session.Spec.AudioTranscode) {
			args = append(args, "-c:a", "aac", "-profile:a", "aac_low", "-b:a", "256k", "-ac", "2", "-ar", "48000")
		} else {
			args = append(args, "-c:a", "copy")
		}
	}
	if strings.EqualFold(session.Spec.VideoCodec, "hevc") || strings.EqualFold(session.Spec.VideoCodec, "h265") {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args,
		"-avoid_negative_ts", "make_zero",
		"-max_muxing_queue_size", "4096",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%.3f", segmentSeconds),
		"-hls_list_size", "0",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", initName,
		"-hls_segment_options", "movflags=+frag_discont+skip_sidx",
		"-hls_flags", "split_by_time+temp_file",
		"-start_number", strconv.Itoa(startIndex),
		"-hls_segment_filename", segmentPattern,
		"-y", playlistPath,
	)

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			resultErr = context.Canceled
			return
		}
		msg := strings.TrimSpace(string(output))
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
		}
		resultErr = fmt.Errorf("ffmpeg hls batch failed: %s", msg)
		return
	}
	_ = os.Remove(playlistPath)
	if err := m.cleanupPressure(0); err != nil {
		log.Printf("stormflix hls cache pressure cleanup: %v", err)
	}
}

func (m *HLSManager) estimateBatchBytes(session *hlsSession, duration float64) int64 {
	bitrate := session.Spec.SourceBitrateKbps
	if bitrate <= 0 {
		bitrate = 50_000
	}
	bytes := float64(bitrate) * 1000 / 8 * duration * 1.15
	if bytes < 8<<20 {
		bytes = 8 << 20
	}
	return int64(bytes)
}

func (m *HLSManager) ensureCapacity(session *hlsSession, batchDuration float64) error {
	estimated := m.estimateBatchBytes(session, batchDuration)
	if err := m.cleanupPressure(estimated); err != nil {
		return err
	}
	usage, _, err := hlsDiskUsage(m.dir)
	if err != nil {
		return err
	}
	free, total, err := diskSpace(m.dir)
	if err != nil {
		return nil
	}
	reserve := m.policy.MinFreeBytes
	if m.policy.MinFreePercent > 0 && total > 0 {
		percent := total * int64(m.policy.MinFreePercent) / 100
		if percent > reserve {
			reserve = percent
		}
	}
	if m.policy.MaxBytes > 0 && usage+estimated > m.policy.MaxBytes {
		return fmt.Errorf("hls cache limit would be exceeded: usage=%d estimate=%d max=%d", usage, estimated, m.policy.MaxBytes)
	}
	if free < reserve+estimated {
		return fmt.Errorf("not enough free disk for hls batch: free=%d reserve=%d estimate=%d", free, reserve, estimated)
	}
	return nil
}

func (m *HLSManager) trimSession(session *hlsSession, focusSegment int) {
	keepFrom := focusSegment - 2
	if keepFrom <= 0 {
		return
	}
	items, err := os.ReadDir(session.Dir)
	if err != nil {
		return
	}
	for _, item := range items {
		name := item.Name()
		if item.IsDir() {
			continue
		}
		if strings.HasPrefix(name, "seg-") && strings.HasSuffix(name, ".m4s") {
			raw := strings.TrimSuffix(strings.TrimPrefix(name, "seg-"), ".m4s")
			index, err := strconv.Atoi(raw)
			if err == nil && index < keepFrom {
				_ = os.Remove(filepath.Join(session.Dir, name))
			}
			continue
		}
		if strings.HasPrefix(name, "init-") && strings.HasSuffix(name, ".mp4") {
			raw := strings.TrimSuffix(strings.TrimPrefix(name, "init-"), ".mp4")
			batch, err := strconv.Atoi(raw)
			if err == nil && batch+m.policy.BatchSegments <= keepFrom {
				_ = os.Remove(filepath.Join(session.Dir, name))
			}
		}
	}
}

func (m *HLSManager) Cleanup(ctx context.Context) error {
	now := time.Now()
	m.mu.Lock()
	stale := make([]*hlsSession, 0)
	for id, session := range m.sessions {
		if now.Sub(session.LastTouch) >= m.policy.IdleTTL {
			delete(m.sessions, id)
			stale = append(stale, session)
		}
	}
	m.mu.Unlock()
	for _, session := range stale {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		m.stopSession(session)
		_ = os.RemoveAll(session.Dir)
	}
	return m.cleanupPressure(0)
}

type hlsCandidate struct {
	path string
	size int64
	mod  time.Time
}

// cleanupPressure evicts only disposable files belonging to sessions without a
// running FFmpeg batch. extraBytes reserves space for the batch that is about
// to start, which makes MaxBytes a hard global budget rather than a threshold
// noticed only after the SSD has already grown.
func (m *HLSManager) cleanupPressure(extraBytes int64) error {
	if extraBytes < 0 {
		extraBytes = 0
	}
	usage, files, err := hlsDiskUsage(m.dir)
	if err != nil {
		return err
	}
	free, total, diskErr := diskSpace(m.dir)
	reserve := m.policy.MinFreeBytes
	if diskErr == nil && m.policy.MinFreePercent > 0 && total > 0 {
		percent := total * int64(m.policy.MinFreePercent) / 100
		if percent > reserve {
			reserve = percent
		}
	}

	need := int64(0)
	if m.policy.MaxBytes > 0 && usage+extraBytes > m.policy.MaxBytes {
		target := m.policy.MaxBytes * int64(m.policy.EvictionTargetPct) / 100
		if target+extraBytes > m.policy.MaxBytes {
			target = m.policy.MaxBytes - extraBytes
		}
		if target < 0 {
			return fmt.Errorf("single hls batch estimate exceeds global cache limit: estimate=%d max=%d", extraBytes, m.policy.MaxBytes)
		}
		need = usage - target
		if need < 0 {
			need = 0
		}
	}
	if diskErr == nil && free < reserve+extraBytes && reserve+extraBytes-free > need {
		need = reserve + extraBytes - free
	}
	if need <= 0 {
		return nil
	}

	m.mu.Lock()
	workers := map[string]bool{}
	for id, session := range m.sessions {
		session.mu.Lock()
		workers[id] = session.worker != nil
		session.mu.Unlock()
	}
	m.mu.Unlock()

	candidates := make([]hlsCandidate, 0, files)
	_ = filepath.WalkDir(m.dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || path == m.dir {
			return nil
		}
		rel, err := filepath.Rel(m.dir, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 || workers[parts[0]] || strings.HasSuffix(entry.Name(), ".tmp") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		candidates = append(candidates, hlsCandidate{path: path, size: info.Size(), mod: info.ModTime()})
		return nil
	})
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mod.Before(candidates[j].mod) })
	removed := int64(0)
	for _, candidate := range candidates {
		if removed >= need {
			break
		}
		if err := os.Remove(candidate.path); err == nil || errors.Is(err, os.ErrNotExist) {
			removed += candidate.size
		}
	}
	if removed < need {
		return fmt.Errorf("hls cache pressure could not free enough space: needed=%d removed=%d", need, removed)
	}
	return nil
}

func hlsDiskUsage(dir string) (int64, int, error) {
	var bytes int64
	files := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		bytes += info.Size()
		files++
		return nil
	})
	return bytes, files, err
}

func (m *HLSManager) Status() HLSStatus {
	usage, files, _ := hlsDiskUsage(m.dir)
	free, _, _ := diskSpace(m.dir)
	m.mu.Lock()
	defer m.mu.Unlock()
	workers := 0
	for _, session := range m.sessions {
		session.mu.Lock()
		if session.worker != nil {
			workers++
		}
		session.mu.Unlock()
	}
	return HLSStatus{
		Directory: m.dir, UsageBytes: usage, MaxBytes: m.policy.MaxBytes, Sessions: len(m.sessions), Workers: workers, Files: files,
		FreeBytes: free, MinFreeBytes: m.policy.MinFreeBytes, MinFreePercent: m.policy.MinFreePercent,
		SegmentSeconds: int(m.policy.SegmentDuration / time.Second), BatchSegments: m.policy.BatchSegments, IdleTTLSeconds: int64(m.policy.IdleTTL / time.Second),
	}
}
