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
	"github.com/danilostorm/stormflix/internal/webcompat"
)

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

	var in playback.Request
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

	// Reuse the Phase 13 technical snapshot when it matches modified_unix. This
	// keeps PlaybackPlan identical while avoiding another ffprobe/rclone read for
	// media the background technical indexer has already inspected.
	source, err := s.probeMediaSource(r.Context(), id, item.Path, item.ModifiedUnix)
	if err != nil {
		writeJSON(w, http.StatusOK, playback.Plan{
			Available:  false,
			Mode:       playback.ModeUnsupported,
			ReasonCode: "source_probe_failed",
			Reason:     err.Error(),
			MediaID:    id,
			ClientKind: in.ClientKind,
		})
		return
	}
	plan := playback.DecideForClient(source, in)
	plan.MediaID = id
	plan.ResumePositionSeconds = resumePosition
	if plan.Available {
		plan.PlaybackSessionID = normalizePlaybackSessionID(in.PlaybackSessionID)
		if plan.PlaybackSessionID == "" {
			plan.PlaybackSessionID = newPlaybackSessionID()
		}

		if strings.EqualFold(strings.TrimSpace(in.ClientKind), "web") {
			if plan.Mode == playback.ModeDirectPlay {
				// A source/version switch can keep the same logical playback session.
				// If the previous source used HLS, Direct Play must discard those
				// fragments immediately instead of leaving them for idle cleanup.
				s.hlsCache.CloseSession(u.ID, plan.PlaybackSessionID)
				plan.URL, plan.PrepareURL = playbackExecutionURLs(id, plan)
			} else {
				hlsAudioTranscode := webcompat.NeedsHLSAAC(plan.AudioCodec, plan.AudioTranscode)
				spec := webcompat.HLSSpec{
					VideoStream:       plan.VideoStream,
					AudioStream:       plan.AudioStream,
					VideoCodec:        plan.VideoCodec,
					AudioCodec:        plan.AudioCodec,
					SourceAudioCodec:  plan.SourceAudioCodec,
					AudioTranscode:    hlsAudioTranscode,
					DurationSeconds:   source.DurationSeconds,
					SourceBitrateKbps: plan.SourceBitrateKbps,
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
				}
			}
		} else {
			plan.URL, plan.PrepareURL = playbackExecutionURLs(id, plan)
		}
	}
	writeJSON(w, http.StatusOK, plan)
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
	if value == "" || len(value) > 128 {
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
