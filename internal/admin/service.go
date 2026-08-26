package admin

import (
	"context"
	"database/sql"
)

type Dashboard struct {
	Users            int64 `json:"users"`
	Libraries        int64 `json:"libraries"`
	Media            int64 `json:"media"`
	ActiveSessions   int64 `json:"active_sessions"`
	ActivePlaybacks  int64 `json:"active_playbacks"`
	OfflineLibraries int64 `json:"offline_libraries"`
}
type LogEntry struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"`
	Category  string `json:"category"`
	Message   string `json:"message"`
	UserID    *int64 `json:"user_id"`
	Details   string `json:"details"`
	CreatedAt string `json:"created_at"`
}
type Playback struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	MediaID     int64  `json:"media_id"`
	Title       string `json:"title"`
	Device      string `json:"device"`
	IP          string `json:"ip"`
	StartedAt   string `json:"started_at"`
	LastSeenAt  string `json:"last_seen_at"`
}
type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	queries := []struct {
		q string
		p *int64
	}{
		{`SELECT COUNT(*) FROM users`, &d.Users},
		{`SELECT COUNT(*) FROM libraries`, &d.Libraries},
		{`SELECT COUNT(*) FROM media WHERE available=1`, &d.Media},
		{`SELECT COUNT(*) FROM sessions WHERE expires_at>CURRENT_TIMESTAMP`, &d.ActiveSessions},
		{`SELECT COUNT(*) FROM playback_sessions WHERE last_seen_at>=datetime('now','-2 minutes')`, &d.ActivePlaybacks},
	}
	for _, v := range queries {
		if err := s.db.QueryRowContext(ctx, v.q).Scan(v.p); err != nil {
			return d, err
		}
	}
	return d, nil
}
func (s *Service) Log(ctx context.Context, level, category, message string, userID *int64, details string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO activity_logs(level,category,message,user_id,details) VALUES(?,?,?,?,?)`, level, category, message, userID, details)
}
func (s *Service) Logs(ctx context.Context, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,level,category,message,user_id,details,created_at FROM activity_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogEntry
	for rows.Next() {
		var v LogEntry
		if err := rows.Scan(&v.ID, &v.Level, &v.Category, &v.Message, &v.UserID, &v.Details, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) TouchPlayback(ctx context.Context, userID, mediaID int64, device, ip string) {
	device = normalizePlaybackDevice(device)
	_, _ = s.db.ExecContext(ctx, `INSERT INTO playback_sessions(user_id,media_id,device,ip) VALUES(?,?,?,?) ON CONFLICT(user_id,media_id,device) DO UPDATE SET ip=excluded.ip,last_seen_at=CURRENT_TIMESTAMP`, userID, mediaID, device, ip)
}
func (s *Service) Playbacks(ctx context.Context) ([]Playback, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.user_id,u.username,u.display_name,p.media_id,m.title,p.device,p.ip,p.started_at,p.last_seen_at FROM playback_sessions p JOIN users u ON u.id=p.user_id JOIN media m ON m.id=p.media_id WHERE p.last_seen_at>=datetime('now','-2 minutes') ORDER BY p.last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Playback
	for rows.Next() {
		var v Playback
		if err := rows.Scan(&v.ID, &v.UserID, &v.Username, &v.DisplayName, &v.MediaID, &v.Title, &v.Device, &v.IP, &v.StartedAt, &v.LastSeenAt); err != nil {
			return nil, err
		}
		v.Device = normalizePlaybackDevice(v.Device)
		out = append(out, v)
	}
	return out, rows.Err()
}
