package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/streaming"
	"github.com/danilostorm/stormflix/internal/transcode"
)

type playbackTelemetryInput struct {
	PlaybackSessionID   string                   `json:"playback_session_id"`
	Mode                string                   `json:"mode"`
	ClientKind          string                   `json:"client_kind"`
	BitrateKbps         int64                    `json:"bitrate_kbps"`
	BufferSeconds       float64                  `json:"buffer_seconds"`
	ReadMbps            float64                  `json:"read_mbps"`
	VideoCodec          string                   `json:"video_codec"`
	AudioCodec          string                   `json:"audio_codec"`
	LastError           string                   `json:"last_error"`
	PlanMS              float64                  `json:"plan_ms"`
	FirstFrameMS        float64                  `json:"first_frame_ms"`
	StartupMS           float64                  `json:"startup_ms"`
	StallCount          int64                    `json:"stall_count"`
	LastStallMS         float64                  `json:"last_stall_ms"`
	Operation           string                   `json:"operation,omitempty"`
	PlaybackPreferences *playbackPreferenceState `json:"playback_preferences,omitempty"`
	Marker              *playbackMarkerState     `json:"marker,omitempty"`
}

func (s *server) playbackTelemetry(w http.ResponseWriter, r *http.Request) {
	mediaID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in playbackTelemetryInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.PlaybackSessionID = normalizePlaybackSessionID(in.PlaybackSessionID)
	in.Mode = strings.ToLower(strings.TrimSpace(in.Mode))
	if in.Mode == "" {
		in.Mode = "direct_play"
	}
	if in.BufferSeconds < 0 {
		in.BufferSeconds = 0
	}
	if in.BufferSeconds > 600 {
		in.BufferSeconds = 600
	}
	if in.ReadMbps < 0 {
		in.ReadMbps = 0
	}
	if in.ReadMbps > 100000 {
		in.ReadMbps = 100000
	}
	if in.BitrateKbps < 0 {
		in.BitrateKbps = 0
	}
	if len(in.LastError) > 500 {
		in.LastError = in.LastError[:500]
	}
	in.PlanMS = boundedMetric(in.PlanMS, 300000)
	in.FirstFrameMS = boundedMetric(in.FirstFrameMS, 300000)
	in.StartupMS = boundedMetric(in.StartupMS, 300000)
	in.LastStallMS = boundedMetric(in.LastStallMS, 300000)
	if in.StallCount < 0 {
		in.StallCount = 0
	} else if in.StallCount > 1000000 {
		in.StallCount = 1000000
	}

	u := currentUser(r)
	item, err := s.media.GetStreamItem(r.Context(), mediaID)
	if err != nil || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if roleLevel(u.Role) < 2 && !containsInt64(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	if s.handlePlaybackDelightTelemetry(w, r, mediaID, in) {
		return
	}

	cacheBytes := int64(0)
	ahead := 0
	if in.PlaybackSessionID != "" && in.Mode != "direct_play" {
		if streaming.IsSessionID(in.PlaybackSessionID) {
			if manager, managerErr := streaming.ForDataDir(s.config.DataDir); managerErr == nil {
				manager.Touch(u.ID, in.PlaybackSessionID)
				for _, info := range manager.Sessions() {
					if info.ID == in.PlaybackSessionID && info.UserID == u.ID {
						cacheBytes = info.CacheBytes
						ahead = info.AheadSegments
						break
					}
				}
			}
		} else if transcode.IsSessionID(in.PlaybackSessionID) {
			if manager, managerErr := transcode.ForDataDir(s.config.DataDir); managerErr == nil {
				manager.Touch(u.ID, in.PlaybackSessionID)
				for _, info := range manager.Sessions() {
					if info.ID == in.PlaybackSessionID && info.UserID == u.ID {
						cacheBytes = info.CacheBytes
						break
					}
				}
			}
		} else if diagnostics, tuneErr := s.hlsCache.TuneSession(u.ID, in.PlaybackSessionID, in.BufferSeconds, in.ReadMbps); tuneErr == nil {
			cacheBytes = diagnostics.CacheBytes
			ahead = diagnostics.AheadBatches
		}
	}
	device := shortDevice(r.UserAgent())
	_, _ = s.db.ExecContext(r.Context(), `
INSERT INTO playback_sessions(user_id,media_id,device,ip,playback_session_id,mode,client_kind,bitrate_kbps,buffer_seconds,read_mbps,cache_bytes,video_codec,audio_codec,last_error,plan_ms,first_frame_ms,startup_ms,stall_count,last_stall_ms)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(user_id,media_id,device) DO UPDATE SET
 ip=excluded.ip,playback_session_id=excluded.playback_session_id,mode=excluded.mode,client_kind=excluded.client_kind,bitrate_kbps=excluded.bitrate_kbps,
	buffer_seconds=excluded.buffer_seconds,read_mbps=excluded.read_mbps,cache_bytes=excluded.cache_bytes,video_codec=excluded.video_codec,audio_codec=excluded.audio_codec,last_error=excluded.last_error,
	plan_ms=excluded.plan_ms,first_frame_ms=excluded.first_frame_ms,startup_ms=excluded.startup_ms,stall_count=excluded.stall_count,last_stall_ms=excluded.last_stall_ms,last_seen_at=CURRENT_TIMESTAMP`,
		u.ID, mediaID, device, clientIP(r), in.PlaybackSessionID, in.Mode, in.ClientKind, in.BitrateKbps, in.BufferSeconds, in.ReadMbps, cacheBytes, in.VideoCodec, in.AudioCodec, in.LastError,
		in.PlanMS, in.FirstFrameMS, in.StartupMS, in.StallCount, in.LastStallMS)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cache_bytes": cacheBytes, "ahead_batches": ahead})
}

func (s *server) playbackDiagnostics(w http.ResponseWriter, r *http.Request) {
	transcodeBySession := map[string]transcode.SessionInfo{}
	streamBySession := map[string]streaming.SessionInfo{}
	if manager, managerErr := transcode.ForDataDir(s.config.DataDir); managerErr == nil {
		for _, info := range manager.Sessions() {
			transcodeBySession[info.ID] = info
		}
	}
	if manager, managerErr := streaming.ForDataDir(s.config.DataDir); managerErr == nil {
		for _, info := range manager.Sessions() {
			streamBySession[info.ID] = info
		}
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT p.id,p.user_id,u.username,u.display_name,p.media_id,m.title,p.device,p.ip,p.started_at,p.last_seen_at,
COALESCE(p.playback_session_id,''),COALESCE(p.mode,'direct_play'),COALESCE(p.client_kind,''),COALESCE(p.bitrate_kbps,0),COALESCE(p.buffer_seconds,0),COALESCE(p.read_mbps,0),COALESCE(p.cache_bytes,0),COALESCE(p.video_codec,''),COALESCE(p.audio_codec,''),COALESCE(p.last_error,''),COALESCE(p.plan_ms,0),COALESCE(p.first_frame_ms,0),COALESCE(p.startup_ms,0),COALESCE(p.stall_count,0),COALESCE(p.last_stall_ms,0)
FROM playback_sessions p JOIN users u ON u.id=p.user_id JOIN media m ON m.id=p.media_id
WHERE p.last_seen_at>=datetime('now','-2 minutes') ORDER BY p.last_seen_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, userID, mediaID, bitrate, cacheBytes int64
		var username, displayName, title, device, ip, startedAt, lastSeenAt, sessionID, mode, clientKind, videoCodec, audioCodec, lastError string
		var bufferSeconds, readMbps float64
		var planMS, firstFrameMS, startupMS, lastStallMS float64
		var stallCount int64
		if err := rows.Scan(&id, &userID, &username, &displayName, &mediaID, &title, &device, &ip, &startedAt, &lastSeenAt, &sessionID, &mode, &clientKind, &bitrate, &bufferSeconds, &readMbps, &cacheBytes, &videoCodec, &audioCodec, &lastError, &planMS, &firstFrameMS, &startupMS, &stallCount, &lastStallMS); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		item := map[string]any{
			"id": id, "user_id": userID, "username": username, "display_name": displayName, "media_id": mediaID, "title": title,
			"device": device, "ip": ip, "started_at": startedAt, "last_seen_at": lastSeenAt, "playback_session_id": sessionID,
			"mode": mode, "client_kind": clientKind, "bitrate_kbps": bitrate, "buffer_seconds": bufferSeconds, "read_mbps": readMbps,
			"cache_bytes": cacheBytes, "video_codec": videoCodec, "audio_codec": audioCodec, "last_error": lastError,
			"plan_ms": planMS, "first_frame_ms": firstFrameMS, "startup_ms": startupMS, "stall_count": stallCount, "last_stall_ms": lastStallMS,
		}
		if trans, ok := transcodeBySession[sessionID]; ok {
			item["transcode"] = trans
			item["cache_bytes"] = trans.CacheBytes
			item["encoder"] = trans.Encoder
			item["hardware"] = trans.Hardware
			item["transcode_fps"] = trans.FPS
			item["transcode_speed"] = trans.Speed
			item["tone_map"] = trans.ToneMap
			item["target_width"] = trans.TargetWidth
			item["target_height"] = trans.TargetHeight
			item["target_bitrate_kbps"] = trans.TargetBitrateKbps
		} else if stream, ok := streamBySession[sessionID]; ok {
			item["stream"] = stream
			item["cache_bytes"] = stream.CacheBytes
			item["encoder"] = stream.Encoder
			item["hardware"] = stream.Hardware
			item["process_id"] = stream.ProcessID
			item["worker_paused"] = stream.WorkerPaused
			item["ahead_segments"] = stream.AheadSegments
			item["server_route"] = stream.Route
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func boundedMetric(value, maximum float64) float64 {
	if value < 0 || value != value {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

func containsInt64(values []int64, wanted int64) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var errForbidden = &httpErrorText{"library access denied"}

type httpErrorText struct{ text string }

func (e *httpErrorText) Error() string { return e.text }
