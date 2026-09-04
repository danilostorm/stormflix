package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/music"
	appsettings "github.com/danilostorm/stormflix/internal/settings"
	"github.com/danilostorm/stormflix/internal/workload"
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
	if in.CompatCacheMaxBytes != nil && *in.CompatCacheMaxBytes < 0 {
		writeError(w, http.StatusBadRequest, errors.New("compatibility cache limit cannot be negative"))
		return
	}
	if in.CompatCacheTTLHours != nil && *in.CompatCacheTTLHours < 0 {
		writeError(w, http.StatusBadRequest, errors.New("compatibility cache TTL cannot be negative"))
		return
	}
	if in.CompatCacheMinFreeBytes != nil && *in.CompatCacheMinFreeBytes < 0 {
		writeError(w, http.StatusBadRequest, errors.New("minimum free disk reserve cannot be negative"))
		return
	}
	if in.CompatCacheMinFreePercent != nil && (*in.CompatCacheMinFreePercent < 0 || *in.CompatCacheMinFreePercent > 95) {
		writeError(w, http.StatusBadRequest, errors.New("minimum free disk percent must be between 0 and 95"))
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
	music.ConfigureProviders(effective.LastFMAPIKey)
	s.compatCache.Configure(compatCachePolicy(effective))
	s.config = effective
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "settings", "Runtime settings updated", &uid, "agents, asset storage and playback cache policy reloaded without restart")
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
	if width, parseErr := strconv.Atoi(r.URL.Query().Get("w")); parseErr == nil && width > 0 {
		active, _ := workload.For(s.db).Active(r.Context())
		if !active {
			variantContext, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			if variant, contentType, ok := s.assets.Variant(variantContext, key, width, r.Header.Get("Accept")); ok {
				path = variant
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Vary", "Accept")
			}
		}
	}
	// Local /assets is authenticated, so keep shared caches out while allowing
	// the browser to reuse artwork aggressively. A configured AssetPublicBaseURL
	// remains the path for an external CDN and can apply its own public policy.
	w.Header().Set("Cache-Control", "private, max-age=86400, stale-while-revalidate=604800")
	http.ServeFile(w, r, path)
}
