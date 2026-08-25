package admin

import "context"

type MonitoringRank struct {
	Label        string `json:"label"`
	Plays        int64  `json:"plays"`
	WatchSeconds int64  `json:"watch_seconds"`
}

type MonitoringDay struct {
	Date         string `json:"date"`
	Plays        int64  `json:"plays"`
	WatchSeconds int64  `json:"watch_seconds"`
}

type MonitoringAnalytics struct {
	TopUsers     []MonitoringRank `json:"top_users"`
	TopMedia     []MonitoringRank `json:"top_media"`
	TopLibraries []MonitoringRank `json:"top_libraries"`
	Daily        []MonitoringDay  `json:"daily"`
}

func (s *Service) MonitoringAnalytics(ctx context.Context) (MonitoringAnalytics, error) {
	var out MonitoringAnalytics
	var err error
	out.TopUsers, err = s.monitoringRank(ctx, `SELECT u.display_name,COUNT(*),CAST(COALESCE(SUM(h.watch_seconds),0) AS INTEGER)
FROM playback_history h JOIN users u ON u.id=h.user_id
WHERE h.stopped_at>=datetime('now','-7 days') GROUP BY h.user_id,u.display_name ORDER BY SUM(h.watch_seconds) DESC,COUNT(*) DESC LIMIT 8`)
	if err != nil { return out, err }
	out.TopMedia, err = s.monitoringRank(ctx, `SELECT m.title,COUNT(*),CAST(COALESCE(SUM(h.watch_seconds),0) AS INTEGER)
FROM playback_history h JOIN media m ON m.id=h.media_id
WHERE h.stopped_at>=datetime('now','-7 days') GROUP BY h.media_id,m.title ORDER BY SUM(h.watch_seconds) DESC,COUNT(*) DESC LIMIT 8`)
	if err != nil { return out, err }
	out.TopLibraries, err = s.monitoringRank(ctx, `SELECT l.name,COUNT(*),CAST(COALESCE(SUM(h.watch_seconds),0) AS INTEGER)
FROM playback_history h JOIN media m ON m.id=h.media_id JOIN libraries l ON l.id=m.library_id
WHERE h.stopped_at>=datetime('now','-7 days') GROUP BY l.id,l.name ORDER BY SUM(h.watch_seconds) DESC,COUNT(*) DESC LIMIT 8`)
	if err != nil { return out, err }
	out.Daily, err = s.monitoringDaily(ctx)
	if err != nil { return out, err }
	if out.TopUsers == nil { out.TopUsers = []MonitoringRank{} }
	if out.TopMedia == nil { out.TopMedia = []MonitoringRank{} }
	if out.TopLibraries == nil { out.TopLibraries = []MonitoringRank{} }
	if out.Daily == nil { out.Daily = []MonitoringDay{} }
	return out, nil
}

func (s *Service) monitoringRank(ctx context.Context, query string) ([]MonitoringRank, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []MonitoringRank{}
	for rows.Next() {
		var item MonitoringRank
		if err := rows.Scan(&item.Label, &item.Plays, &item.WatchSeconds); err != nil { return nil, err }
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) monitoringDaily(ctx context.Context) ([]MonitoringDay, error) {
	rows, err := s.db.QueryContext(ctx, `WITH RECURSIVE days(day) AS (
SELECT date('now','-6 days') UNION ALL SELECT date(day,'+1 day') FROM days WHERE day<date('now')
)
SELECT days.day,COUNT(h.id),CAST(COALESCE(SUM(h.watch_seconds),0) AS INTEGER)
FROM days LEFT JOIN playback_history h ON date(h.stopped_at)=days.day GROUP BY days.day ORDER BY days.day`)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []MonitoringDay{}
	for rows.Next() {
		var item MonitoringDay
		if err := rows.Scan(&item.Date, &item.Plays, &item.WatchSeconds); err != nil { return nil, err }
		out = append(out, item)
	}
	return out, rows.Err()
}
