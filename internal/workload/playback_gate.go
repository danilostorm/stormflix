// Package workload coordinates expensive background work with active playback.
package workload

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

const defaultPollInterval = 2 * time.Second

// Gate makes catalog/indexing workers yield while video or game playback is
// active. A gate is shared per database so Admin diagnostics see every worker.
type Gate struct {
	db   *sql.DB
	poll time.Duration

	mu             sync.Mutex
	activeWaiters  map[string]int
	totalPauses    uint64
	totalPaused    time.Duration
	lastPauseAt    time.Time
	lastResumeAt   time.Time
	lastQueryError string
	categoryPauses map[string]uint64
	categoryPaused map[string]time.Duration
}

type Snapshot struct {
	PlaybackActive   bool               `json:"playback_active"`
	ActiveWaiters    map[string]int     `json:"active_waiters"`
	TotalPauses      uint64             `json:"total_pauses"`
	TotalPausedMS    float64            `json:"total_paused_ms"`
	LastPauseAt      string             `json:"last_pause_at,omitempty"`
	LastResumeAt     string             `json:"last_resume_at,omitempty"`
	LastQueryError   string             `json:"last_query_error,omitempty"`
	CategoryPauses   map[string]uint64  `json:"category_pauses"`
	CategoryPausedMS map[string]float64 `json:"category_paused_ms"`
}

var registry sync.Map

func For(db *sql.DB) *Gate {
	if db == nil {
		return &Gate{poll: defaultPollInterval, activeWaiters: map[string]int{}, categoryPauses: map[string]uint64{}, categoryPaused: map[string]time.Duration{}}
	}
	if existing, ok := registry.Load(db); ok {
		return existing.(*Gate)
	}
	gate := &Gate{db: db, poll: defaultPollInterval, activeWaiters: map[string]int{}, categoryPauses: map[string]uint64{}, categoryPaused: map[string]time.Duration{}}
	actual, _ := registry.LoadOrStore(db, gate)
	return actual.(*Gate)
}

// Active reads only recent heartbeats. Stale rows never keep workers paused.
func (g *Gate) Active(ctx context.Context) (bool, error) {
	if g == nil || g.db == nil {
		return false, nil
	}
	var active int
	err := g.db.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM playback_sessions WHERE last_seen_at>=datetime('now','-90 seconds'))+
 (SELECT COUNT(*) FROM game_play_sessions WHERE last_seen_at>=datetime('now','-45 seconds'))`).Scan(&active)
	if err != nil {
		g.mu.Lock()
		g.lastQueryError = err.Error()
		g.mu.Unlock()
		return false, err
	}
	g.mu.Lock()
	g.lastQueryError = ""
	g.mu.Unlock()
	return active > 0, nil
}

// Wait blocks an expensive worker only while fresh playback heartbeats exist.
// Query failures fail open: a damaged diagnostics query must not deadlock every
// persistent job. onState is called once when pausing and once when resuming.
func (g *Gate) Wait(ctx context.Context, category string, onState func(paused bool)) error {
	if category == "" {
		category = "background"
	}
	paused := false
	var pausedAt time.Time
	defer func() {
		if paused {
			g.finishPause(category, pausedAt)
			if onState != nil {
				onState(false)
			}
		}
	}()
	for {
		active, err := g.Active(ctx)
		if err != nil || !active {
			if paused {
				g.finishPause(category, pausedAt)
				paused = false
				if onState != nil {
					onState(false)
				}
			}
			return ctx.Err()
		}
		if !paused {
			paused = true
			pausedAt = time.Now()
			g.beginPause(category, pausedAt)
			if onState != nil {
				onState(true)
			}
		}
		timer := time.NewTimer(g.poll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *Gate) beginPause(category string, at time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.activeWaiters[category]++
	g.totalPauses++
	g.categoryPauses[category]++
	g.lastPauseAt = at.UTC()
}

func (g *Gate) finishPause(category string, started time.Time) {
	duration := time.Since(started)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activeWaiters[category] > 1 {
		g.activeWaiters[category]--
	} else {
		delete(g.activeWaiters, category)
	}
	g.totalPaused += duration
	g.categoryPaused[category] += duration
	g.lastResumeAt = time.Now().UTC()
}

func (g *Gate) Snapshot(ctx context.Context) Snapshot {
	active, _ := g.Active(ctx)
	g.mu.Lock()
	defer g.mu.Unlock()
	out := Snapshot{
		PlaybackActive:   active,
		ActiveWaiters:    make(map[string]int, len(g.activeWaiters)),
		TotalPauses:      g.totalPauses,
		TotalPausedMS:    float64(g.totalPaused.Microseconds()) / 1000,
		LastQueryError:   g.lastQueryError,
		CategoryPauses:   make(map[string]uint64, len(g.categoryPauses)),
		CategoryPausedMS: make(map[string]float64, len(g.categoryPaused)),
	}
	if !g.lastPauseAt.IsZero() {
		out.LastPauseAt = g.lastPauseAt.Format(time.RFC3339)
	}
	if !g.lastResumeAt.IsZero() {
		out.LastResumeAt = g.lastResumeAt.Format(time.RFC3339)
	}
	for category, count := range g.activeWaiters {
		out.ActiveWaiters[category] = count
	}
	for category, count := range g.categoryPauses {
		out.CategoryPauses[category] = count
	}
	for category, duration := range g.categoryPaused {
		out.CategoryPausedMS[category] = float64(duration.Microseconds()) / 1000
	}
	return out
}
