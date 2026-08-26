package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/danilostorm/stormflix/internal/auth"
)

const profileCookie = "stormflix_profile"

type profileInput struct {
	Name               string `json:"name"`
	AvatarKey          string `json:"avatar_key"`
	AvatarURL          string `json:"avatar_url"`
	IsKids             bool   `json:"is_kids"`
	ContentRatingLimit *int   `json:"content_rating_limit,omitempty"`
	Active             *bool  `json:"active,omitempty"`
	PIN                string `json:"pin,omitempty"`
	ClearPIN           bool   `json:"clear_pin,omitempty"`
	AutoplayNext       *bool  `json:"autoplay_next,omitempty"`
	AutoplayPreviews   *bool  `json:"autoplay_previews,omitempty"`
	PreferredAudio     string `json:"preferred_audio,omitempty"`
	PreferredSubtitle  string `json:"preferred_subtitle,omitempty"`
}

func (s *server) listProfiles(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	profiles, err := s.auth.Profiles(r.Context(), u.ID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	selected := s.selectedProfileID(r, u.ID)
	writeJSON(w, 200, map[string]any{"profiles": profiles, "selected_profile_id": selected})
}

func (s *server) createOwnProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	p, err := s.decodeCreateProfile(r, u.ID)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 201, p)
}

func (s *server) updateOwnProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	p, err := s.decodeUpdateProfile(r, u.ID, id)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *server) deleteOwnProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if err := s.auth.DeleteProfile(r.Context(), u.ID, id); err != nil {
		writeError(w, 400, err)
		return
	}
	if s.selectedProfileID(r, u.ID) == id {
		clearProfileCookie(w)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) selectProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	p, err := s.auth.Profile(r.Context(), u.ID, id)
	if errors.Is(err, sql.ErrNoRows) || !p.Active {
		writeError(w, 404, errors.New("profile not found"))
		return
	}
	if err != nil {
		writeError(w, 500, err)
		return
	}
	var in struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, 400, err)
		return
	}
	if p.PINEnabled {
		if err := s.auth.VerifyProfilePIN(r.Context(), u.ID, id, in.PIN); err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: strconv.FormatInt(p.ID, 10), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r), MaxAge: 365 * 24 * 3600})
	writeJSON(w, 200, p)
}

func (s *server) adminUserProfiles(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	profiles, err := s.auth.Profiles(r.Context(), userID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, profiles)
}

func (s *server) adminCreateProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	p, err := s.decodeCreateProfile(r, userID)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 201, p)
}

func (s *server) adminUpdateProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var userID int64
	if err := s.db.QueryRowContext(r.Context(), `SELECT user_id FROM profiles WHERE id=?`, profileID).Scan(&userID); err != nil {
		writeError(w, 404, err)
		return
	}
	p, err := s.decodeUpdateProfile(r, userID, profileID)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, p)
}

func (s *server) adminDeleteProfile(w http.ResponseWriter, r *http.Request) {
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	var userID int64
	if err := s.db.QueryRowContext(r.Context(), `SELECT user_id FROM profiles WHERE id=?`, profileID).Scan(&userID); err != nil {
		writeError(w, 404, err)
		return
	}
	if err := s.auth.DeleteProfile(r.Context(), userID, profileID); err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) decodeCreateProfile(r *http.Request, userID int64) (auth.Profile, error) {
	var in profileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return auth.Profile{}, err
	}
	p, err := s.auth.CreateProfile(r.Context(), userID, in.Name, in.AvatarKey, in.AvatarURL, in.IsKids)
	if err != nil {
		return auth.Profile{}, err
	}
	if in.ContentRatingLimit != nil {
		p, err = s.auth.SetProfileRatingLimit(r.Context(), userID, p.ID, *in.ContentRatingLimit)
		if err != nil {
			return auth.Profile{}, err
		}
	}
	if in.PIN != "" || in.ClearPIN || in.AutoplayNext != nil || in.AutoplayPreviews != nil || in.PreferredAudio != "" || in.PreferredSubtitle != "" {
		next := p.AutoplayNext
		previews := p.AutoplayPreviews
		if in.AutoplayNext != nil {
			next = *in.AutoplayNext
		}
		if in.AutoplayPreviews != nil {
			previews = *in.AutoplayPreviews
		}
		audio := p.PreferredAudio
		subtitle := p.PreferredSubtitle
		if in.PreferredAudio != "" {
			audio = in.PreferredAudio
		}
		if in.PreferredSubtitle != "" {
			subtitle = in.PreferredSubtitle
		}
		p, err = s.auth.UpdateProfilePreferences(r.Context(), userID, p.ID, in.PIN, in.ClearPIN, next, previews, audio, subtitle)
	}
	return p, err
}

func (s *server) decodeUpdateProfile(r *http.Request, userID, profileID int64) (auth.Profile, error) {
	var in profileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return auth.Profile{}, err
	}
	current, err := s.auth.Profile(r.Context(), userID, profileID)
	if err != nil {
		return auth.Profile{}, err
	}
	active := current.Active
	if in.Active != nil {
		active = *in.Active
	}
	p, err := s.auth.UpdateProfile(r.Context(), userID, profileID, in.Name, in.AvatarKey, in.AvatarURL, in.IsKids, active)
	if err != nil {
		return auth.Profile{}, err
	}
	limit := current.ContentRatingLimit
	if in.ContentRatingLimit != nil {
		limit = *in.ContentRatingLimit
	} else if in.IsKids && !current.IsKids && current.ContentRatingLimit >= 18 {
		limit = 10
	} else if !in.IsKids && current.IsKids && current.ContentRatingLimit <= 10 {
		limit = 18
	}
	p, err = s.auth.SetProfileRatingLimit(r.Context(), userID, profileID, limit)
	if err != nil {
		return auth.Profile{}, err
	}
	next := current.AutoplayNext
	previews := current.AutoplayPreviews
	if in.AutoplayNext != nil {
		next = *in.AutoplayNext
	}
	if in.AutoplayPreviews != nil {
		previews = *in.AutoplayPreviews
	}
	audio := current.PreferredAudio
	subtitle := current.PreferredSubtitle
	if in.PreferredAudio != "" {
		audio = in.PreferredAudio
	}
	if in.PreferredSubtitle != "" {
		subtitle = in.PreferredSubtitle
	}
	return s.auth.UpdateProfilePreferences(r.Context(), userID, profileID, in.PIN, in.ClearPIN, next, previews, audio, subtitle)
}

func (s *server) selectedProfileID(r *http.Request, userID int64) int64 {
	cookie, err := r.Cookie(profileCookie)
	if err != nil {
		return 0
	}
	id, err := strconv.ParseInt(cookie.Value, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM profiles WHERE id=? AND user_id=? AND active=1`, id, userID).Scan(&exists); err != nil || exists == 0 {
		return 0
	}
	return id
}

func clearProfileCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}
