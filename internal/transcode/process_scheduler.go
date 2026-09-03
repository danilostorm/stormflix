package transcode

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ProcessScheduler bounds all FFmpeg processes created by the playback
// engines. Remux/audio-copy work is comparatively cheap but still consumes IO
// and page cache; video encodes additionally use the smaller video semaphore.
type ProcessScheduler struct {
	total chan struct{}
	video chan struct{}

	totalLimit  int
	videoLimit  int
	active      atomic.Int64
	videoActive atomic.Int64
	waiters     atomic.Int64
}

type ProcessResourceStatus struct {
	TotalLimit     int   `json:"total_limit"`
	VideoLimit     int   `json:"video_limit"`
	CPUThreadLimit int   `json:"cpu_thread_limit"`
	Active         int64 `json:"active"`
	VideoActive    int64 `json:"video_active"`
	Waiters        int64 `json:"waiters"`
}

func NewProcessScheduler(totalLimit, videoLimit int) *ProcessScheduler {
	if totalLimit < 1 {
		totalLimit = 4
	}
	if videoLimit < 1 {
		videoLimit = 2
	}
	if videoLimit > totalLimit {
		videoLimit = totalLimit
	}
	return &ProcessScheduler{
		total: make(chan struct{}, totalLimit), video: make(chan struct{}, videoLimit),
		totalLimit: totalLimit, videoLimit: videoLimit,
	}
}

func (s *ProcessScheduler) Acquire(ctx context.Context, video bool) (func(), time.Duration, error) {
	started := time.Now()
	s.waiters.Add(1)
	defer s.waiters.Add(-1)
	if video {
		select {
		case s.video <- struct{}{}:
		case <-ctx.Done():
			return nil, time.Since(started), ctx.Err()
		}
	}
	select {
	case s.total <- struct{}{}:
	case <-ctx.Done():
		if video {
			<-s.video
		}
		return nil, time.Since(started), ctx.Err()
	}
	s.active.Add(1)
	if video {
		s.videoActive.Add(1)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			<-s.total
			s.active.Add(-1)
			if video {
				<-s.video
				s.videoActive.Add(-1)
			}
		})
	}
	return release, time.Since(started), nil
}

func (s *ProcessScheduler) Status() ProcessResourceStatus {
	return ProcessResourceStatus{
		TotalLimit: s.totalLimit, VideoLimit: s.videoLimit,
		Active: s.active.Load(), VideoActive: s.videoActive.Load(), Waiters: s.waiters.Load(),
	}
}

var globalProcessScheduler = struct {
	sync.RWMutex
	value *ProcessScheduler
}{value: NewProcessScheduler(4, 2)}

var cpuThreadLimit atomic.Int64

func ConfigureProcessScheduler(totalLimit, videoLimit int) {
	globalProcessScheduler.Lock()
	defer globalProcessScheduler.Unlock()
	// Configuration is applied during server construction. Never replace a live
	// scheduler, because doing so would make existing release callbacks account
	// against a different semaphore.
	if globalProcessScheduler.value.Status().Active > 0 {
		return
	}
	globalProcessScheduler.value = NewProcessScheduler(totalLimit, videoLimit)
}

func ConfigureCPUThreadLimit(limit int) {
	if limit < 1 {
		limit = 6
	}
	if available := runtime.NumCPU(); available > 0 && limit > available {
		limit = available
	}
	cpuThreadLimit.Store(int64(limit))
}

func CPUThreadLimit() int {
	limit := int(cpuThreadLimit.Load())
	if limit < 1 {
		limit = 6
		if available := runtime.NumCPU(); available > 0 && limit > available {
			limit = available
		}
	}
	return limit
}

func AcquireProcess(ctx context.Context, video bool) (func(), time.Duration, error) {
	globalProcessScheduler.RLock()
	scheduler := globalProcessScheduler.value
	globalProcessScheduler.RUnlock()
	return scheduler.Acquire(ctx, video)
}

func ProcessResources() ProcessResourceStatus {
	globalProcessScheduler.RLock()
	scheduler := globalProcessScheduler.value
	globalProcessScheduler.RUnlock()
	status := scheduler.Status()
	status.CPUThreadLimit = CPUThreadLimit()
	return status
}
