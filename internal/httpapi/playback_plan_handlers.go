package httpapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/playback"
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

	source, err := playback.Probe(r.Context(), item.Path)
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
		plan.PlaybackSessionID = newPlaybackSessionID()
		switch plan.Mode {
		case playback.ModeDirectPlay:
			plan.URL = fmt.Sprintf("/api/v1/media/%d/stream", id)
		case playback.ModeRemux:
			plan.URL = fmt.Sprintf("/api/v1/media/%d/remux", id)
			plan.PrepareURL = fmt.Sprintf("/api/v1/media/%d/remux/prepare", id)
		case playback.ModeAudioCompatibility:
			plan.URL = fmt.Sprintf("/api/v1/media/%d/remux?audio=aac", id)
			plan.PrepareURL = fmt.Sprintf("/api/v1/media/%d/remux/prepare?audio=aac", id)
		}
	}
	writeJSON(w, http.StatusOK, plan)
}

func newPlaybackSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("sf-%d", time.Now().UnixNano())
}
