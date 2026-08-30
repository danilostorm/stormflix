package webcompat

import (
	"errors"
	"os"
	"sync"
	"time"
)

type adaptiveHLSState struct {
	Ahead   int
	Touched time.Time
}

var hlsAdaptiveAhead sync.Map // session id -> adaptiveHLSState

type HLSSessionDiagnostics struct {
	SessionID         string  `json:"session_id"`
	MediaID           int64   `json:"media_id"`
	CacheBytes        int64   `json:"cache_bytes"`
	BufferSeconds     float64 `json:"buffer_seconds"`
	ReadMbps          float64 `json:"read_mbps"`
	SourceBitrateKbps int64   `json:"source_bitrate_kbps"`
	AheadBatches      int     `json:"ahead_batches"`
}

// TuneSession changes only speculative headroom. The global cache budget and
// free-disk reserve remain hard limits enforced by ensureCapacity, so hundreds
// of users can never make this adaptive policy grow the SSD without bound.
func (m *HLSManager) TuneSession(userID int64, sessionID string, bufferSeconds, readMbps float64) (HLSSessionDiagnostics, error) {
	m.mu.Lock()
	session := m.sessions[sessionID]
	if session == nil || session.UserID != userID || session.Closed {
		m.mu.Unlock()
		return HLSSessionDiagnostics{}, errors.New("HLS session not found")
	}
	session.LastTouch = time.Now()
	m.mu.Unlock()

	ahead := 1
	sourceMbps := float64(session.Spec.SourceBitrateKbps) / 1000.0
	slowRead := readMbps > 0 && sourceMbps > 0 && readMbps < sourceMbps*1.35
	switch {
	case bufferSeconds < 10 || slowRead:
		ahead = 3
	case bufferSeconds < 22:
		ahead = 2
	default:
		ahead = 1
	}
	if bufferSeconds > 40 {
		ahead = 1
	}
	storeAdaptiveAhead(sessionID, ahead)
	return HLSSessionDiagnostics{
		SessionID: sessionID, MediaID: session.MediaID, CacheBytes: hlsSessionDirSize(session.Dir),
		BufferSeconds: bufferSeconds, ReadMbps: readMbps, SourceBitrateKbps: session.Spec.SourceBitrateKbps, AheadBatches: ahead,
	}, nil
}

func storeAdaptiveAhead(sessionID string, ahead int) {
	now := time.Now()
	if ahead < 1 {
		ahead = 1
	}
	if ahead > 3 {
		ahead = 3
	}
	hlsAdaptiveAhead.Store(sessionID, adaptiveHLSState{Ahead: ahead, Touched: now})
	// Normal HLS directories are removed immediately/TTL by HLSManager. The
	// telemetry hint map is process memory, so prune abandoned session keys
	// opportunistically as well and keep long-running servers bounded.
	cutoff := now.Add(-2 * time.Hour)
	hlsAdaptiveAhead.Range(func(key, value any) bool {
		state, ok := value.(adaptiveHLSState)
		if !ok || state.Touched.Before(cutoff) {
			hlsAdaptiveAhead.Delete(key)
		}
		return true
	})
}

func adaptiveAheadBatches(sessionID string) int {
	if value, ok := hlsAdaptiveAhead.Load(sessionID); ok {
		state, ok := value.(adaptiveHLSState)
		if !ok || state.Touched.Before(time.Now().Add(-2*time.Hour)) {
			hlsAdaptiveAhead.Delete(sessionID)
			return 1
		}
		if state.Ahead < 1 {
			return 1
		}
		if state.Ahead > 3 {
			return 3
		}
		return state.Ahead
	}
	return 1
}

func (m *HLSManager) SessionDiagnostics(userID int64, sessionID string) (HLSSessionDiagnostics, error) {
	m.mu.Lock()
	session := m.sessions[sessionID]
	if session == nil || session.UserID != userID || session.Closed {
		m.mu.Unlock()
		return HLSSessionDiagnostics{}, errors.New("HLS session not found")
	}
	mediaID := session.MediaID
	dir := session.Dir
	bitrate := session.Spec.SourceBitrateKbps
	m.mu.Unlock()
	return HLSSessionDiagnostics{SessionID: sessionID, MediaID: mediaID, CacheBytes: hlsSessionDirSize(dir), SourceBitrateKbps: bitrate, AheadBatches: adaptiveAheadBatches(sessionID)}, nil
}

func hlsSessionDirSize(root string) int64 {
	var total int64
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}
