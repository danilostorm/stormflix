package admin

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
)

type PlaybackHeartbeat struct {
	PositionSeconds  float64 `json:"position_seconds"`
	DurationSeconds  float64 `json:"duration_seconds"`
	State            string  `json:"state"`
	Mode             string  `json:"mode"`
	Resolution       string  `json:"resolution"`
	VideoCodec       string  `json:"video_codec"`
	AudioCodec       string  `json:"audio_codec"`
	SourceAudioCodec string  `json:"source_audio_codec"`
	AudioLanguage    string  `json:"audio_language"`
	SubtitleLanguage string  `json:"subtitle_language"`
	BitrateKbps      int64   `json:"bitrate_kbps"`
}

type MonitoringPlayback struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Username         string  `json:"username"`
	DisplayName      string  `json:"display_name"`
	MediaID          int64   `json:"media_id"`
	Title            string  `json:"title"`
	LibraryName      string  `json:"library_name"`
	MediaKind        string  `json:"media_kind"`
	Artist           string  `json:"artist"`
	Album            string  `json:"album"`
	PosterURL        string  `json:"poster_url"`
	Device           string  `json:"device"`
	IP               string  `json:"ip"`
	State            string  `json:"state"`
	Mode             string  `json:"mode"`
	PositionSeconds  float64 `json:"position_seconds"`
	DurationSeconds  float64 `json:"duration_seconds"`
	ProgressPercent  float64 `json:"progress_percent"`
	Resolution       string  `json:"resolution"`
	VideoCodec       string  `json:"video_codec"`
	AudioCodec       string  `json:"audio_codec"`
	SourceAudioCodec string  `json:"source_audio_codec"`
	AudioLanguage    string  `json:"audio_language"`
	SubtitleLanguage string  `json:"subtitle_language"`
	BitrateKbps      int64   `json:"bitrate_kbps"`
	StartedAt        string  `json:"started_at"`
	LastSeenAt       string  `json:"last_seen_at"`
}

type PlaybackHistory struct {
	ID              int64   `json:"id"`
	UserID          int64   `json:"user_id"`
	DisplayName     string  `json:"display_name"`
	MediaID         int64   `json:"media_id"`
	Title           string  `json:"title"`
	LibraryName     string  `json:"library_name"`
	Device          string  `json:"device"`
	IP              string  `json:"ip"`
	Mode            string  `json:"mode"`
	ProgressPercent float64 `json:"progress_percent"`
	WatchSeconds    float64 `json:"watch_seconds"`
	StartedAt       string  `json:"started_at"`
	StoppedAt       string  `json:"stopped_at"`
}

type MonitoringStats struct {
	ActiveStreams     int64 `json:"active_streams"`
	DirectPlayStreams int64 `json:"direct_play_streams"`
	AudioAACStreams   int64 `json:"audio_aac_streams"`
	WebRemuxStreams   int64 `json:"web_remux_streams"`
	MusicStreams      int64 `json:"music_streams"`
	PausedStreams     int64 `json:"paused_streams"`
	BandwidthKbps     int64 `json:"bandwidth_kbps"`
	PlaysToday        int64 `json:"plays_today"`
	Plays7Days        int64 `json:"plays_7_days"`
	UniqueUsers7Days  int64 `json:"unique_users_7_days"`
	WatchSeconds7Days int64 `json:"watch_seconds_7_days"`
}

type MonitoringOverview struct {
	Stats   MonitoringStats      `json:"stats"`
	Active  []MonitoringPlayback `json:"active"`
	History []PlaybackHistory    `json:"history"`
}

func EnsureMonitoring(db *sql.DB) error {
	columns := []struct{ name, definition string }{
		{"state", "TEXT NOT NULL DEFAULT 'playing'"},
		{"mode", "TEXT NOT NULL DEFAULT 'direct_play'"},
		{"position_seconds", "REAL NOT NULL DEFAULT 0"},
		{"duration_seconds", "REAL NOT NULL DEFAULT 0"},
		{"resolution", "TEXT NOT NULL DEFAULT ''"},
		{"video_codec", "TEXT NOT NULL DEFAULT ''"},
		{"audio_codec", "TEXT NOT NULL DEFAULT ''"},
		{"source_audio_codec", "TEXT NOT NULL DEFAULT ''"},
		{"audio_language", "TEXT NOT NULL DEFAULT ''"},
		{"subtitle_language", "TEXT NOT NULL DEFAULT ''"},
		{"bitrate_kbps", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		if err := ensurePlaybackColumn(db, column.name, column.definition); err != nil {
			return err
		}
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS playback_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		media_id INTEGER NOT NULL,
		device TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		mode TEXT NOT NULL DEFAULT 'direct_play',
		progress_percent REAL NOT NULL DEFAULT 0,
		watch_seconds REAL NOT NULL DEFAULT 0,
		started_at TEXT NOT NULL,
		stopped_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_playback_history_stopped ON playback_history(stopped_at);
	CREATE INDEX IF NOT EXISTS idx_playback_history_user ON playback_history(user_id,stopped_at);
	CREATE INDEX IF NOT EXISTS idx_playback_history_media ON playback_history(media_id,stopped_at);`)
	return err
}

func ensurePlaybackColumn(db *sql.DB, name, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(playback_sessions)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if columnName == name {
			found = true
			break
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE playback_sessions ADD COLUMN %s %s`, name, definition))
	return err
}

func normalizePlaybackDevice(device string) string {
	device = strings.TrimSpace(device)
	lower := strings.ToLower(device)
	family := strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(lower)
	if strings.Contains(family, "stormflix android") {
		return "StormFlix Android"
	}
	if strings.Contains(family, "stormflix desktop") {
		return "StormFlix Desktop"
	}
	return device
}

func (s *Service) Heartbeat(ctx context.Context, userID, mediaID int64, device, ip string, hb PlaybackHeartbeat) error {
	device = normalizePlaybackDevice(device)
	mode := normalizeMode(hb.Mode)
	state := strings.ToLower(strings.TrimSpace(hb.State))
	if state != "paused" {
		state = "playing"
	}
	bitrate := hb.BitrateKbps
	if bitrate <= 0 && hb.DurationSeconds > 0 {
		var size int64
		if err := s.db.QueryRowContext(ctx, `SELECT size_bytes FROM media WHERE id=?`, mediaID).Scan(&size); err == nil && size > 0 {
			bitrate = int64(math.Round(float64(size*8) / hb.DurationSeconds / 1000))
		}
	}
	audioCodec := strings.TrimSpace(hb.AudioCodec)
	if audioCodec == "" && mode == "audio_aac" {
		audioCodec = "aac"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO playback_sessions(user_id,media_id,device,ip,state,mode,position_seconds,duration_seconds,resolution,video_codec,audio_codec,source_audio_codec,audio_language,subtitle_language,bitrate_kbps)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(user_id,media_id,device) DO UPDATE SET
	ip=excluded.ip,
	state=excluded.state,
	mode=excluded.mode,
	position_seconds=excluded.position_seconds,
	duration_seconds=CASE WHEN excluded.duration_seconds>0 THEN excluded.duration_seconds ELSE playback_sessions.duration_seconds END,
	resolution=CASE WHEN excluded.resolution<>'' THEN excluded.resolution ELSE playback_sessions.resolution END,
	video_codec=CASE WHEN excluded.video_codec<>'' THEN excluded.video_codec ELSE playback_sessions.video_codec END,
	audio_codec=CASE WHEN excluded.audio_codec<>'' THEN excluded.audio_codec ELSE playback_sessions.audio_codec END,
	source_audio_codec=CASE WHEN excluded.source_audio_codec<>'' THEN excluded.source_audio_codec ELSE playback_sessions.source_audio_codec END,
	audio_language=CASE WHEN excluded.audio_language<>'' THEN excluded.audio_language ELSE playback_sessions.audio_language END,
	subtitle_language=CASE WHEN excluded.subtitle_language<>'' THEN excluded.subtitle_language ELSE playback_sessions.subtitle_language END,
	bitrate_kbps=CASE WHEN excluded.bitrate_kbps>0 THEN excluded.bitrate_kbps ELSE playback_sessions.bitrate_kbps END,
	last_seen_at=CURRENT_TIMESTAMP`,
		userID, mediaID, device, ip, state, mode, hb.PositionSeconds, hb.DurationSeconds, strings.TrimSpace(hb.Resolution), strings.TrimSpace(hb.VideoCodec), audioCodec, strings.TrimSpace(hb.SourceAudioCodec), strings.TrimSpace(hb.AudioLanguage), strings.TrimSpace(hb.SubtitleLanguage), bitrate)
	return err
}

func (s *Service) FinishPlayback(ctx context.Context, userID, mediaID int64, device string) error {
	device = normalizePlaybackDevice(device)
	var id int64
	var ip, mode, started string
	var position, duration float64
	err := s.db.QueryRowContext(ctx, `SELECT id,ip,mode,position_seconds,duration_seconds,started_at FROM playback_sessions WHERE user_id=? AND media_id=? AND device=?`, userID, mediaID, device).Scan(&id, &ip, &mode, &position, &duration, &started)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	progress := 0.0
	if duration > 0 {
		progress = math.Min(100, position/duration*100)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO playback_history(user_id,media_id,device,ip,mode,progress_percent,watch_seconds,started_at) VALUES(?,?,?,?,?,?,?,?)`, userID, mediaID, device, ip, normalizeMode(mode), progress, math.Max(0, position), started); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM playback_sessions WHERE id=?`, id)
	return err
}

func (s *Service) Monitoring(ctx context.Context) (MonitoringOverview, error) {
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
		switch p.Mode {
		case "web_remux":
			out.Stats.WebRemuxStreams++
		case "audio_aac":
			out.Stats.AudioAACStreams++
		case "music":
			out.Stats.MusicStreams++
			out.Stats.DirectPlayStreams++
		default:
			out.Stats.DirectPlayStreams++
		}
	}
	queries := []struct {
		q   string
		dst *int64
	}{
		{`SELECT COUNT(*) FROM playback_history WHERE stopped_at>=datetime('now','start of day')`, &out.Stats.PlaysToday},
		{`SELECT COUNT(*) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`, &out.Stats.Plays7Days},
		{`SELECT COUNT(DISTINCT user_id) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`, &out.Stats.UniqueUsers7Days},
		{`SELECT COALESCE(SUM(watch_seconds),0) FROM playback_history WHERE stopped_at>=datetime('now','-7 days')`, &out.Stats.WatchSeconds7Days},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.q).Scan(query.dst); err != nil {
			return out, err
		}
	}
	out.History, err = s.monitoringHistory(ctx, 80)
	if err != nil {
		return out, err
	}
	if out.History == nil {
		out.History = []PlaybackHistory{}
	}
	return out, nil
}

func (s *Service) archiveStalePlaybacks(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO playback_history(user_id,media_id,device,ip,mode,progress_percent,watch_seconds,started_at,stopped_at)
SELECT user_id,media_id,device,ip,mode,
CASE WHEN duration_seconds>0 THEN MIN(100,position_seconds/duration_seconds*100) ELSE 0 END,
MAX(0,position_seconds),started_at,last_seen_at
FROM playback_sessions WHERE last_seen_at<datetime('now','-2 minutes')`)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM playback_sessions WHERE last_seen_at<datetime('now','-2 minutes')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) monitoringActive(ctx context.Context) ([]MonitoringPlayback, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.user_id,u.username,u.display_name,p.media_id,
COALESCE(NULLIF(mt.title,''),m.title),l.name,l.kind,COALESCE(mt.artist,''),COALESCE(mt.album,''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
p.device,p.ip,p.state,p.mode,p.position_seconds,
COALESCE(NULLIF(p.duration_seconds,0),mt.duration_seconds,0),p.resolution,p.video_codec,
COALESCE(NULLIF(p.audio_codec,''),mt.codec,''),COALESCE(NULLIF(p.source_audio_codec,''),mt.codec,''),
p.audio_language,p.subtitle_language,
COALESCE(NULLIF(p.bitrate_kbps,0),CAST(mt.bitrate/1000 AS INTEGER),0),p.started_at,p.last_seen_at
FROM playback_sessions p
JOIN users u ON u.id=p.user_id
JOIN media m ON m.id=p.media_id
JOIN libraries l ON l.id=m.library_id
LEFT JOIN music_tracks mt ON mt.media_id=m.id
WHERE p.last_seen_at>=datetime('now','-75 seconds') ORDER BY p.last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MonitoringPlayback{}
	seen := map[string]bool{}
	for rows.Next() {
		var v MonitoringPlayback
		if err := rows.Scan(&v.ID, &v.UserID, &v.Username, &v.DisplayName, &v.MediaID, &v.Title, &v.LibraryName, &v.MediaKind, &v.Artist, &v.Album, &v.PosterURL, &v.Device, &v.IP, &v.State, &v.Mode, &v.PositionSeconds, &v.DurationSeconds, &v.Resolution, &v.VideoCodec, &v.AudioCodec, &v.SourceAudioCodec, &v.AudioLanguage, &v.SubtitleLanguage, &v.BitrateKbps, &v.StartedAt, &v.LastSeenAt); err != nil {
			return nil, err
		}
		v.Device = normalizePlaybackDevice(v.Device)
		key := fmt.Sprintf("%d:%d:%s:%s", v.UserID, v.MediaID, v.IP, strings.ToLower(v.Device))
		if seen[key] {
			continue
		}
		seen[key] = true
		if strings.EqualFold(v.MediaKind, "music") {
			v.Mode = "music"
		}
		if v.DurationSeconds > 0 {
			v.ProgressPercent = math.Min(100, v.PositionSeconds/v.DurationSeconds*100)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) monitoringHistory(ctx context.Context, limit int) ([]PlaybackHistory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT h.id,h.user_id,u.display_name,h.media_id,COALESCE(NULLIF(mt.title,''),m.title),l.name,h.device,h.ip,h.mode,h.progress_percent,h.watch_seconds,h.started_at,h.stopped_at
FROM playback_history h JOIN users u ON u.id=h.user_id JOIN media m ON m.id=h.media_id JOIN libraries l ON l.id=m.library_id
LEFT JOIN music_tracks mt ON mt.media_id=m.id ORDER BY h.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlaybackHistory{}
	for rows.Next() {
		var v PlaybackHistory
		if err := rows.Scan(&v.ID, &v.UserID, &v.DisplayName, &v.MediaID, &v.Title, &v.LibraryName, &v.Device, &v.IP, &v.Mode, &v.ProgressPercent, &v.WatchSeconds, &v.StartedAt, &v.StoppedAt); err != nil {
			return nil, err
		}
		v.Device = normalizePlaybackDevice(v.Device)
		out = append(out, v)
	}
	return out, rows.Err()
}

func normalizeMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "music") {
		return "music"
	}
	if strings.Contains(value, "aac") {
		return "audio_aac"
	}
	if strings.Contains(value, "remux") {
		return "web_remux"
	}
	return "direct_play"
}
