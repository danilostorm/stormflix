package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
	"github.com/danilostorm/stormflix/internal/streaming"
	"github.com/danilostorm/stormflix/internal/transcode"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

type playbackPlanRequest struct {
	playback.Request
	AudioStream          *int     `json:"audio_stream,omitempty"`
	StartPositionSeconds *float64 `json:"start_position_seconds,omitempty"`
}

func (s *server) playbackPlan(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return
	}
	item, err := s.media.GetStreamItem(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || !item.Available {
		writeError(w, http.StatusNotFound, errors.New("media not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if roleLevel(u.Role) < 2 && !media.ContainsLibrary(u.LibraryIDs, item.LibraryID) {
		writeError(w, http.StatusForbidden, errors.New("library access denied"))
		return
	}

	var in playbackPlanRequest
	if decodeJSON(w, r, &in) != nil {
		return
	}

	profileID := s.selectedProfileID(r, u.ID)
	resumePosition := 0.0
	if profileID > 0 {
		if profile, profileErr := s.auth.Profile(r.Context(), u.ID, profileID); profileErr == nil && in.PreferredAudioLanguage == "" {
			in.PreferredAudioLanguage = profile.PreferredAudio
		}
		err = s.db.QueryRowContext(r.Context(), `SELECT position_seconds FROM profile_progress WHERE profile_id=? AND media_id=?`, profileID, id).Scan(&resumePosition)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	source, err := s.probeMediaSource(r.Context(), id, item.Path, item.ModifiedUnix)
	if err != nil {
		writeJSON(w, http.StatusOK, playback.Plan{Available: false, Mode: playback.ModeUnsupported, ReasonCode: "source_probe_failed", Reason: err.Error(), MediaID: id, ClientKind: in.ClientKind})
		return
	}
	if in.StartPositionSeconds != nil {
		resumePosition = *in.StartPositionSeconds
		if resumePosition < 0 {
			resumePosition = 0
		}
		if source.DurationSeconds > 0 && resumePosition >= source.DurationSeconds {
			resumePosition = source.DurationSeconds - 2
			if resumePosition < 0 {
				resumePosition = 0
			}
		}
	}

	plan := playback.DecideForClient(source, in.Request)
	if in.AudioStream != nil {
		plan = playback.ApplyAudioStream(source, in.Request, plan, *in.AudioStream)
	}
	plan.MediaID = id
	plan.ResumePositionSeconds = resumePosition
	if !plan.Available {
		writeJSON(w, http.StatusOK, plan)
		return
	}

	clientKind := strings.ToLower(strings.TrimSpace(in.ClientKind))
	webContinuous := clientKind == "web" || clientKind == "desktop"
	previousSession := normalizePlaybackSessionID(in.PlaybackSessionID)

	// Web v5.3 follows a Jellyfin-style session model: Direct Play stays a raw
	// HTTP stream, while every route that needs server mediation gets one
	// long-running FFmpeg job for the playback session instead of one process per
	// HLS batch. Android/TV deliberately stay on the existing transport for now.
	if webContinuous {
		if plan.Mode == playback.ModeDirectPlay {
			s.closePlaybackTransport(u.ID, previousSession)
			plan.PlaybackSessionID = newPlaybackSessionID()
			plan.URL, plan.PrepareURL = playbackExecutionURLs(id, plan)
			plan.FallbackURL = ""
			plan.FallbackPrepareURL = ""
			writeJSON(w, http.StatusOK, plan)
			return
		}

		manager, managerErr := streaming.ForDataDir(s.config.DataDir)
		if managerErr != nil {
			plan.Available = false
			plan.Mode = playback.ModeUnsupported
			plan.ReasonCode = "web_session_engine_unavailable"
			plan.Reason = managerErr.Error()
			writeJSON(w, http.StatusOK, plan)
			return
		}
		engine := transcode.Detect()
		if engine.FFmpegPath == "" {
			plan.Available = false
			plan.Mode = playback.ModeUnsupported
			plan.ReasonCode = "ffmpeg_unavailable"
			plan.Reason = "ffmpeg is not installed"
			writeJSON(w, http.StatusOK, plan)
			return
		}
		if plan.Mode == playback.ModeVideoTranscode {
			plan.Encoder = preferredEncoder(engine, plan.VideoCodec)
			if plan.Encoder == "" {
				plan.Available = false
				plan.Mode = playback.ModeUnsupported
				plan.ReasonCode = "video_encoder_unavailable"
				plan.Reason = "no compatible video encoder is available for browser playback"
				writeJSON(w, http.StatusOK, plan)
				return
			}
			plan.HardwareAcceleration = encoderHardware(plan.Encoder)
		}

		s.closePlaybackTransport(u.ID, previousSession)
		plan.PlaybackSessionID = streaming.SessionID(newPlaybackSessionID())
		audioTranscode := plan.AudioTranscode || webcompat.NeedsHLSAAC(plan.AudioCodec, false)
		targetAudioCodec := plan.AudioCodec
		if audioTranscode && plan.AudioStream >= 0 {
			targetAudioCodec = "aac"
			plan.AudioCodec = "aac"
			plan.AudioTranscode = true
			if plan.Mode != playback.ModeVideoTranscode {
				plan.Mode = playback.ModeAudioCompatibility
			}
		}
		spec := streaming.Spec{
			VideoStream: plan.VideoStream, AudioStream: plan.AudioStream,
			SourceVideoCodec: plan.SourceVideoCodec, TargetVideoCodec: plan.VideoCodec,
			SourceAudioCodec: plan.SourceAudioCodec, TargetAudioCodec: targetAudioCodec,
			VideoTranscode: plan.Mode == playback.ModeVideoTranscode, AudioTranscode: audioTranscode,
			Width: plan.VideoWidth, Height: plan.VideoHeight, TargetWidth: plan.TargetVideoWidth, TargetHeight: plan.TargetVideoHeight,
			FrameRate: plan.VideoFrameRate, TargetFrameRate: plan.TargetVideoFrameRate, ToneMap: plan.ToneMap,
			TargetBitrateKbps: plan.TargetBitrateKbps, DurationSeconds: source.DurationSeconds,
			StartSeconds: resumePosition, Quality: plan.Quality,
		}
		if err := manager.Prepare(plan.PlaybackSessionID, u.ID, id, item.Path, spec); err != nil {
			plan.Available = false
			plan.Mode = playback.ModeUnsupported
			plan.ReasonCode = "web_session_prepare_failed"
			plan.Reason = err.Error()
			plan.URL = ""
		} else {
			plan.URL = fmt.Sprintf("/api/v1/media/%d/webstream/%s/index.m3u8", id, plan.PlaybackSessionID)
			plan.PrepareURL = ""
			plan.FallbackURL = ""
			plan.FallbackPrepareURL = ""
		}
		writeJSON(w, http.StatusOK, plan)
		return
	}

	plan.PlaybackSessionID = previousSession
	if plan.PlaybackSessionID == "" {
		plan.PlaybackSessionID = newPlaybackSessionID()
	}

	transcoder, transcodeErr := transcode.ForDataDir(s.config.DataDir)
	if transcodeErr != nil && plan.Mode == playback.ModeVideoTranscode {
		plan.Available = false
		plan.Mode = playback.ModeUnsupported
		plan.ReasonCode = "transcode_engine_unavailable"
		plan.Reason = transcodeErr.Error()
		writeJSON(w, http.StatusOK, plan)
		return
	}

	if plan.Mode == playback.ModeDirectPlay {
		if transcode.IsSessionID(plan.PlaybackSessionID) && transcoder != nil {
			transcoder.Close(u.ID, plan.PlaybackSessionID)
			plan.PlaybackSessionID = newPlaybackSessionID()
		}
		s.hlsCache.CloseSession(u.ID, plan.PlaybackSessionID)
		plan.URL, plan.PrepareURL = playbackExecutionURLs(id, plan)
		writeJSON(w, http.StatusOK, plan)
		return
	}

	if !clientUsesDynamicHLS(in.ClientKind) {
		if plan.Mode == playback.ModeVideoTranscode {
			plan.Available = false
			plan.Mode = playback.ModeUnsupported
			plan.ReasonCode = "client_transcode_transport_unsupported"
			plan.Reason = "this legacy client does not advertise the StormFlix dynamic HLS transport required for video transcoding"
		} else {
			plan.URL, plan.PrepareURL = playbackExecutionURLs(id, plan)
		}
		writeJSON(w, http.StatusOK, plan)
		return
	}

	if plan.Mode == playback.ModeVideoTranscode {
		if !transcode.IsSessionID(plan.PlaybackSessionID) {
			s.hlsCache.CloseSession(u.ID, plan.PlaybackSessionID)
			plan.PlaybackSessionID = transcode.SessionID(plan.PlaybackSessionID)
		}
		engine := transcoder.EngineStatus()
		plan.Encoder = preferredEncoder(engine, plan.VideoCodec)
		plan.HardwareAcceleration = encoderHardware(plan.Encoder)
		spec := transcode.Spec{
			VideoStream: plan.VideoStream, AudioStream: plan.AudioStream,
			SourceVideoCodec: plan.SourceVideoCodec, TargetVideoCodec: plan.VideoCodec,
			SourceAudioCodec: plan.SourceAudioCodec, TargetAudioCodec: plan.AudioCodec, AudioTranscode: plan.AudioTranscode,
			Width: plan.VideoWidth, Height: plan.VideoHeight, TargetWidth: plan.TargetVideoWidth, TargetHeight: plan.TargetVideoHeight,
			FrameRate: plan.VideoFrameRate, TargetFrameRate: plan.TargetVideoFrameRate, SourceHDR: plan.VideoHDR, ToneMap: plan.ToneMap,
			TargetBitrateKbps: plan.TargetBitrateKbps, DurationSeconds: source.DurationSeconds, Reason: plan.ReasonCode, Quality: plan.Quality,
		}
		if err := transcoder.Prepare(plan.PlaybackSessionID, u.ID, id, item.Path, spec); err != nil {
			plan.Available = false
			plan.Mode = playback.ModeUnsupported
			plan.ReasonCode = "transcode_session_prepare_failed"
			plan.Reason = err.Error()
			plan.URL = ""
			plan.PrepareURL = ""
		} else {
			plan.URL = fmt.Sprintf("/api/v1/media/%d/hls/%s/index.m3u8", id, plan.PlaybackSessionID)
			plan.PrepareURL = ""
		}
		writeJSON(w, http.StatusOK, plan)
		return
	}

	if transcode.IsSessionID(plan.PlaybackSessionID) {
		if transcoder != nil {
			transcoder.Close(u.ID, plan.PlaybackSessionID)
		}
		plan.PlaybackSessionID = newPlaybackSessionID()
	}
	hlsAudioTranscode := webcompat.NeedsHLSAAC(plan.AudioCodec, plan.AudioTranscode)
	spec := webcompat.HLSSpec{
		VideoStream: plan.VideoStream, AudioStream: plan.AudioStream, VideoCodec: plan.VideoCodec,
		AudioCodec: plan.AudioCodec, SourceAudioCodec: plan.SourceAudioCodec, AudioTranscode: hlsAudioTranscode,
		DurationSeconds: source.DurationSeconds, SourceBitrateKbps: plan.SourceBitrateKbps,
	}
	if err := s.hlsCache.PrepareSession(plan.PlaybackSessionID, u.ID, id, item.Path, spec); err != nil {
		plan.Available = false
		plan.Mode = playback.ModeUnsupported
		plan.ReasonCode = "hls_session_prepare_failed"
		plan.Reason = err.Error()
		plan.URL = ""
		plan.PrepareURL = ""
	} else {
		if hlsAudioTranscode {
			plan.Mode = playback.ModeAudioCompatibility
			plan.AudioTranscode = true
			plan.AudioCodec = "aac"
		}
		plan.URL = fmt.Sprintf("/api/v1/media/%d/hls/%s/index.m3u8", id, plan.PlaybackSessionID)
		plan.PrepareURL = ""
		plan.FallbackURL, plan.FallbackPrepareURL = playbackExecutionURLs(id, plan)
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *server) closePlaybackTransport(userID int64, session string) {
	session = normalizePlaybackSessionID(session)
	if session == "" {
		return
	}
	if streaming.IsSessionID(session) {
		if manager, err := streaming.ForDataDir(s.config.DataDir); err == nil {
			manager.Close(userID, session)
		}
		return
	}
	if transcode.IsSessionID(session) {
		if manager, err := transcode.ForDataDir(s.config.DataDir); err == nil {
			manager.Close(userID, session)
		}
		return
	}
	s.hlsCache.CloseSession(userID, session)
}

func preferredEncoder(status transcode.EngineStatus, codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hevc", "h265":
		return status.PreferredHEVC
	case "av1":
		return status.PreferredAV1
	default:
		return status.PreferredH264
	}
}

func encoderHardware(encoder string) string {
	switch {
	case strings.Contains(encoder, "nvenc"):
		return "nvidia"
	case strings.Contains(encoder, "qsv"):
		return "qsv"
	case strings.Contains(encoder, "vaapi"):
		return "vaapi"
	case encoder != "":
		return "cpu"
	default:
		return ""
	}
}

func clientUsesDynamicHLS(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "web", "desktop", "android", "tv", "android_tv", "firetv", "fire_tv":
		return true
	default:
		return false
	}
}

func playbackExecutionURLs(id int64, plan playback.Plan) (string, string) {
	if plan.Mode == playback.ModeDirectPlay {
		return fmt.Sprintf("/api/v1/media/%d/stream", id), ""
	}
	values := url.Values{}
	if plan.AudioStream >= 0 {
		values.Set("audio_stream", strconv.Itoa(plan.AudioStream))
	}
	if plan.Mode == playback.ModeAudioCompatibility {
		values.Set("audio", "aac")
	}
	suffix := ""
	if encoded := values.Encode(); encoded != "" {
		suffix = "?" + encoded
	}
	return fmt.Sprintf("/api/v1/media/%d/remux%s", id, suffix), fmt.Sprintf("/api/v1/media/%d/remux/prepare%s", id, suffix)
}

func normalizePlaybackSessionID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 132 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return value
}

func newPlaybackSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("sf-%d", time.Now().UnixNano())
}
