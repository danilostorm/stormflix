package webcompat

import (
	"errors"
	"os"
	"sync"
)

var hlsAdaptiveAhead sync.Map // session id -> int (1..3 batches)

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
	session, err := m.getSession(userID, 0, sessionID)
	if err != nil {
		// getSession normally validates media too; telemetry intentionally accepts
		// any media owned by the authenticated user for this session.
		m.mu.Lock()
		session = m.sessions[sessionID]
		if session == nil || session.UserID != userID || session.Closed {
			m.mu.Unlock()
			return HLSSessionDiagnostics{}, errors.New("HLS session not found")
		}
		m.mu.Unlock()
	}

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
	hlsAdaptiveAhead.Store(sessionID, ahead)
	return HLSSessionDiagnostics{
		SessionID: sessionID, MediaID: session.MediaID, CacheBytes: hlsSessionDirSize(session.Dir),
		BufferSeconds: bufferSeconds, ReadMbps: readMbps, SourceBitrateKbps: session.Spec.SourceBitrateKbps, AheadBatches: ahead,
	}, nil
}

func adaptiveAheadBatches(sessionID string) int {
	if value, ok := hlsAdaptiveAhead.Load(sessionID); ok {
		if n, ok := value.(int); ok {
			if n < 1 {
				return 1
			}
			if n > 3 {
				return 3
			}
			return n
		}
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
