package admin

import (
	"context"
	"math"
)

// MonitoringSafe is equivalent to Monitoring, but it scans watch-time totals
// through REAL values first. SQLite returns SUM() over a REAL column as
// float64, which cannot be scanned directly into int64 when fractions exist.
func (s *Service) MonitoringSafe(ctx context.Context) (MonitoringOverview, error) {
	var out MonitoringOverview
	if err := s.archiveStalePlaybacks(ctx); err != nil {
		return out, err
	}
	active, err := s.monitoringActive(ctx)
	if err != nil {
		return out, err
	}
	out.Active = active
	if out.Active == nil {
		out.Active = []MonitoringPlayback{}
	}
	for _, p := range out.Active {
		out.Stats.ActiveStreams++
		out.Stats.BandwidthKbps += p.BitrateKbps
		if p.State == "paused" {
			out.Stats.PausedStreams++
		}
		if p.Mode == "web_remux" {
			out.Stats.WebRemuxStreams++
		} else {
			out.Stats.DirectPlayStreams++
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_history WHERE stopped_at>=datetime('now','start of day')`).Scan(&out.Stats.PlaysToday); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`).Scan(&out.Stats.Plays7Days); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`).Scan(&out.Stats.UniqueUsers7Days); err != nil {
		return out, err
	}
	var watched float64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(watch_seconds),0.0) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`).Scan(&watched); err != nil {
		return out, err
	}
	out.Stats.WatchSeconds7Days = int64(math.Round(watched))
	out.History, err = s.monitoringHistory(ctx, 80)
	if err != nil {
		return out, err
	}
	if out.History == nil {
		out.History = []PlaybackHistory{}
	}
	return out, nil
}
