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

// PrimeSession starts generating the first bounded HLS batch as soon as the
// PlaybackPlan is created. The browser can load hls.js and the manifest while
// FFmpeg is already reading the rclone/Drive source, removing the old cold-start
// round trip where generation only began after the first init/segment request.
func (m *HLSManager) PrimeSession(userID, mediaID int64, sessionID string) {
	session, err := m.getSession(userID, mediaID, sessionID)
	if err != nil {
		return
	}
	key := hlsPrefetchKey{manager: m, sessionID: sessionID, batch: -2}
	if _, loaded := hlsPrefetchInFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	go func(expected *hlsSession) {
		defer hlsPrefetchInFlight.Delete(key)
		current, err := m.getSession(userID, mediaID, sessionID)
		if err != nil || current != expected {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		_ = m.ensureBatch(ctx, current, 0, 0)
	}(session)
}

// SegmentPathBuffered serves the requested HLS fragment and warms a bounded
// adaptive amount ahead. The default remains one batch; telemetry may raise it
// to at most three small batches when the browser buffer or remote read speed
// indicates risk. ensureCapacity still enforces the single global SSD budget.
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
		hlsAdaptiveAhead.Delete(sessionID)
		return
	}
	batchSegments := m.policy.BatchSegments
	maxSegments := int(math.Ceil(session.Spec.DurationSeconds / m.policy.SegmentDuration.Seconds()))
	if _, ok := nextHLSBatchStart(segment, batchSegments, maxSegments); !ok {
		return
	}

	// One speculative chain per session. Browser requests always go through the
	// normal SegmentPath and retain priority over this background worker.
	key := hlsPrefetchKey{manager: m, sessionID: sessionID, batch: -1}
	if _, loaded := hlsPrefetchInFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	session.mu.Lock()
	if session.Closed {
		session.mu.Unlock()
		hlsPrefetchInFlight.Delete(key)
		return
	}
	initialWorker := session.worker
	session.mu.Unlock()

	go func(waitWorker *hlsWorker) {
		defer hlsPrefetchInFlight.Delete(key)
		ahead := adaptiveAheadBatches(sessionID)
		cursor := segment
		for step := 0; step < ahead; step++ {
			nextBatch, ok := nextHLSBatchStart(cursor, batchSegments, maxSegments)
			if !ok {
				return
			}

			// Never cancel the worker producing fragments being consumed now.
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
				if active == waitWorker {
					// runBatch closes done just before clearing this pointer. Taking the
					// completed worker out here avoids a tiny hand-off race.
					current.worker = nil
				} else if nextBatch >= active.start && nextBatch < active.end {
					// A browser request already started exactly the batch we wanted.
					waitWorker = active
					current.mu.Unlock()
					cursor = nextBatch
					continue
				} else {
					// Seek/user work owns the encoder. Speculation yields immediately.
					current.mu.Unlock()
					return
				}
			}
			current.mu.Unlock()

			prefetchCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			err = m.ensureBatch(prefetchCtx, current, nextBatch, nextBatch)
			cancel()
			if err != nil {
				return
			}
			current.mu.Lock()
			waitWorker = current.worker
			current.mu.Unlock()
			cursor = nextBatch
		}
	}(initialWorker)
}
