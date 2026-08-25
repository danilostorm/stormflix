package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/config"
	appsettings "github.com/danilostorm/stormflix/internal/settings"
)

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	out, err := s.settings.Public(r.Context(), s.baseConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in appsettings.Update
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.AssetDir != nil && strings.TrimSpace(*in.AssetDir) == "" {
		writeError(w, http.StatusBadRequest, errors.New("asset directory cannot be empty"))
		return
	}
	if in.MetadataLanguage != nil && strings.TrimSpace(*in.MetadataLanguage) == "" {
		writeError(w, http.StatusBadRequest, errors.New("metadata language cannot be empty"))
		return
	}

	candidateDir := s.config.AssetDir
	candidateURL := s.config.AssetPublicBaseURL
	if in.AssetDir != nil {
		candidateDir = strings.TrimSpace(*in.AssetDir)
	}
	if in.AssetPublicBaseURL != nil {
		candidateURL = strings.TrimSpace(*in.AssetPublicBaseURL)
	}
	if _, err := assets.New(candidateDir, candidateURL); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("asset storage cannot be used: "+err.Error()))
		return
	}

	if err := s.settings.Update(r.Context(), in); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	effective, err := s.settings.Apply(r.Context(), s.baseConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	effective = config.NormalizeCredentials(effective)
	if err := s.assets.Configure(effective.AssetDir, effective.AssetPublicBaseURL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.metadata.Configure(effective)
	s.subtitles.Configure(effective)
	s.config = effective
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "settings", "Runtime settings updated", &uid, "agents and asset storage reloaded without restart")
	out, err := s.settings.Public(r.Context(), s.baseConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) serveAsset(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/assets/")
	path, err := s.assets.Resolve(key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	http.ServeFile(w, r, path)
}
