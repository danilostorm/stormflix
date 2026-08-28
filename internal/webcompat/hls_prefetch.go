package webcompat

import (
	"context"
	"math"
	"sync"
	"time"
)

type hlsPrefetchKey struct {
	manager   *HLSManager
	sessionID string
	batch     int
}

var hlsPrefetchInFlight sync.Map

// SegmentPathBuffered serves the requested HLS fragment and immediately keeps
// one batch ahead warm in the background. The regular SegmentPath remains the
// primitive used by tests/internal callers; Web playback uses this buffered
// variant so a new FFmpeg/rclone read does not begin only after the browser has
// already consumed the final buffered fragment.
func (m *HLSManager) SegmentPathBuffered(ctx context.Context, userID, mediaID int64, sessionID string, segment int) (string, error) {
	path, err := m.SegmentPath(ctx, userID, mediaID, sessionID, segment)
	if err != nil {
		return "", err
	}
	m.prefetchFollowingBatch(userID, mediaID, sessionID, segment)
	return path, nil
}

func nextHLSBatchStart(segment, batchSegments, maxSegments int) (int, bool) {
	if segment < 0 || batchSegments <= 0 || maxSegments <= 0 {
		return 0, false
	}
	currentBatch := (segment / batchSegments) * batchSegments
	nextBatch := currentBatch + batchSegments
	if nextBatch >= maxSegments {
		return 0, false
	}
	return nextBatch, true
}

func (m *HLSManager) prefetchFollowingBatch(userID, mediaID int64, sessionID string, segment int) {
	session, err := m.getSession(userID, mediaID, sessionID)
	if err != nil {
		return
	}
	batchSegments := m.policy.BatchSegments
	maxSegments := int(math.Ceil(session.Spec.DurationSeconds / m.policy.SegmentDuration.Seconds()))
	nextBatch, ok := nextHLSBatchStart(segment, batchSegments, maxSegments)
	if !ok {
		return
	}

	key := hlsPrefetchKey{manager: m, sessionID: sessionID, batch: nextBatch}
	if _, loaded := hlsPrefetchInFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	session.mu.Lock()
	if session.Closed {
		session.mu.Unlock()
		hlsPrefetchInFlight.Delete(key)
		return
	}
	worker := session.worker
	if worker != nil && nextBatch >= worker.start && nextBatch < worker.end {
		session.mu.Unlock()
		hlsPrefetchInFlight.Delete(key)
		return
	}
	session.mu.Unlock()

	go func(waitWorker *hlsWorker) {
		defer hlsPrefetchInFlight.Delete(key)

		// Let the current batch finish first. This avoids cancelling the FFmpeg
		// process that is still producing the fragments the browser is consuming.
		if waitWorker != nil {
			select {
			case <-waitWorker.done:
			case <-time.After(45 * time.Second):
				return
			}
		}

		current, err := m.getSession(userID, mediaID, sessionID)
		if err != nil || current != session {
			return
		}
		current.mu.Lock()
		if current.Closed {
			current.mu.Unlock()
			return
		}
		active := current.worker
		if active != nil {
			// Another request (for example, a seek) has priority. Never cancel an
			// active user-requested batch merely to satisfy speculative prefetch.
			current.mu.Unlock()
			return
		}
		current.mu.Unlock()

		prefetchCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		// ensureBatch starts the worker and returns as soon as the first fragment
		// of the next batch is available. The worker keeps producing the rest of
		// that small batch in the background, maintaining roughly one batch of
		// headroom without materializing the whole movie.
		_ = m.ensureBatch(prefetchCtx, current, nextBatch, nextBatch)
	}(worker)
}
