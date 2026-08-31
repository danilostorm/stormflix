package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"

	appsettings "github.com/danilostorm/stormflix/internal/settings"
	"github.com/danilostorm/stormflix/internal/trakt"
)

var traktIntegrations sync.Map

func (s *server) traktIntegration() *trakt.Service {
	clientID, clientSecret, redirectURI, _, err := s.settings.TraktApplication(context.Background(), s.baseConfig)
	if err != nil {
		clientID, clientSecret, redirectURI = s.config.TraktClientID, s.config.TraktClientSecret, s.config.TraktRedirectURI
	}
	if value, ok := traktIntegrations.Load(s); ok {
		service := value.(*trakt.Service)
		service.Configure(clientID, clientSecret, redirectURI)
		return service
	}
	service := trakt.New(s.db, s.settings, clientID, clientSecret, redirectURI)
	actual, loaded := traktIntegrations.LoadOrStore(s, service)
	if loaded {
		service = actual.(*trakt.Service)
		service.Configure(clientID, clientSecret, redirectURI)
	}
	return service
}

func (s *server) adminTraktSettings(w http.ResponseWriter, r *http.Request) {
	_, _, _, public, err := s.settings.TraktApplication(r.Context(), s.baseConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, public)
}

func (s *server) updateAdminTraktSettings(w http.ResponseWriter, r *http.Request) {
	var in appsettings.TraktApplicationUpdate
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if err := s.settings.UpdateTraktApplication(r.Context(), in); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	clientID, clientSecret, redirectURI, public, err := s.settings.TraktApplication(r.Context(), s.baseConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.traktIntegration().Configure(clientID, clientSecret, redirectURI)
	u := currentUser(r)
	s.admin.Log(r.Context(), "info", "settings", "Trakt application settings updated", &u.ID, "client credentials remain encrypted and profile OAuth tokens stay isolated")
	writeJSON(w, http.StatusOK, public)
}

func (s *server) ownedProfile(w http.ResponseWriter, r *http.Request) (int64, bool) {
	profileID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return 0, false
	}
	u := currentUser(r)
	var exists int
	if err := s.db.QueryRowContext(r.Context(), `SELECT 1 FROM profiles WHERE id=? AND user_id=? AND active=1`, profileID, u.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("profile not found"))
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return 0, false
	}
	return profileID, true
}

func (s *server) profileTraktStatus(w http.ResponseWriter, r *http.Request) {
	profileID, ok := s.ownedProfile(w, r)
	if !ok {
		return
	}
	status, err := s.traktIntegration().Status(r.Context(), profileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) profileTraktDevice(w http.ResponseWriter, r *http.Request) {
	profileID, ok := s.ownedProfile(w, r)
	if !ok {
		return
	}
	status, err := s.traktIntegration().BeginDevice(r.Context(), profileID)
	if errors.Is(err, trakt.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) profileTraktPoll(w http.ResponseWriter, r *http.Request) {
	profileID, ok := s.ownedProfile(w, r)
	if !ok {
		return
	}
	status, err := s.traktIntegration().PollDevice(r.Context(), profileID)
	if errors.Is(err, trakt.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if errors.Is(err, trakt.ErrExpired) {
		writeJSON(w, http.StatusGone, map[string]any{"error": err.Error(), "status": status})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) profileTraktDisconnect(w http.ResponseWriter, r *http.Request) {
	profileID, ok := s.ownedProfile(w, r)
	if !ok {
		return
	}
	if err := s.traktIntegration().Disconnect(r.Context(), profileID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) scrobbleProfilePlayback(profileID, mediaID int64, position, duration float64, state string) {
	if profileID <= 0 || mediaID <= 0 || duration <= 0 {
		return
	}
	var ref trakt.MediaRef
	ref.MediaID = mediaID
	ref.Position = position
	ref.Duration = duration
	ref.PlaybackState = state
	_ = s.db.QueryRow(`
SELECT COALESCE(mm.media_type,''),COALESCE(NULLIF(si.series_title,''),m.title),COALESCE(mm.year,0),
       COALESCE(mm.tmdb_id,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0)
FROM media m
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN media_series_identity si ON si.media_id=m.id
WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&ref.MediaType, &ref.Title, &ref.Year, &ref.TMDBID, &ref.Season, &ref.Episode)
	if ref.TMDBID <= 0 {
		return
	}
	s.traktIntegration().ScrobbleAsync(profileID, ref)
}
