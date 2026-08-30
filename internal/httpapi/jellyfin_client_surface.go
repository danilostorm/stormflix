package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// jellyfinMobileBridge is intentionally a native StormFlix endpoint. The
// official Jellyfin Android application loads the server root in a WebView,
// intercepts main.*.bundle.js to inject its native shell and later watches a
// Sessions/Capabilities/Full request to extract jellyfin_credentials from
// localStorage. StormFlix already authenticates the Web UI with the same token
// stored in its HttpOnly session cookie, so this endpoint exposes that token
// only to the already-authenticated same-origin page. It never accepts a user
// id from the browser and never weakens Jellyfin or native access controls.
func (s *server) jellyfinMobileBridge(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server_id":    s.jellyfinServerID(),
		"server_name":  strings.TrimSpace(s.config.ServerName),
		"user_id":      s.jellyfinExternalID(r.Context(), "user", strconv.FormatInt(u.ID, 10)),
		"access_token": cookie.Value,
		"base_url":     jellyfinRequestBaseURL(r),
		"version":      jellyfinCompatibilityVersion,
	})
}

func jellyfinEmptyQueryResult() map[string]any {
	return map[string]any{
		"Items":            []any{},
		"TotalRecordCount": 0,
		"StartIndex":       0,
	}
}

// The official Android/Android TV clients probe a wider read-only API surface
// than StormFlix needs for its own catalog. Returning correctly shaped empty
// query results for optional features is preferable to malformed DTOs while
// keeping the native /api/v1 model authoritative.
func (s *server) jellyfinEmptyItems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jellyfinEmptyQueryResult())
}

func (s *server) jellyfinEmptyList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (s *server) jellyfinQuickConnectEnabled(w http.ResponseWriter, r *http.Request) {
	// StormFlix does not implement Jellyfin Quick Connect. Explicit false keeps
	// clients on the supported username/password flow instead of advertising a
	// protocol that would later fail.
	writeJSON(w, http.StatusOK, false)
}

func (s *server) jellyfinPublicConfiguration(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"IsStartupWizardCompleted": true,
		"UICulture":                "pt-BR",
		"MetadataCountryCode":      "BR",
		"PreferredMetadataLanguage": "pt-BR",
	})
}

func (s *server) jellyfinPlaybackBitrateTest(w http.ResponseWriter, r *http.Request) {
	// Some Android TV builds call this small endpoint while estimating network
	// throughput. A deterministic binary payload is enough; no media is read.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(make([]byte, 64*1024))
}
