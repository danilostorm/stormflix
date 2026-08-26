package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// This is the version exposed to Jellyfin clients. It intentionally follows
// Jellyfin's numeric server-version grammar (3 numeric components). The native
// StormFlix version remains independent and is still exposed by /healthz.
const jellyfinCompatibilityVersion = "10.11.6"

func (s *server) jellyfinCompatInfo(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"LocalAddress":           "",
		"ServerName":             name,
		"Version":                jellyfinCompatibilityVersion,
		"ProductName":            "Jellyfin Server",
		"OperatingSystem":        "Linux",
		"OperatingSystemDisplayName": "StormFlix",
		"Id":                     s.jellyfinServerID(),
		"StartupWizardCompleted": true,
	})
}

func (s *server) jellyfinCompatSystemInfo(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"LocalAddress":             "",
		"ServerName":               name,
		"Version":                  jellyfinCompatibilityVersion,
		"ProductName":              "Jellyfin Server",
		"OperatingSystem":          "Linux",
		"OperatingSystemDisplayName": "StormFlix",
		"Id":                       s.jellyfinServerID(),
		"StartupWizardCompleted":   true,
		"WanAddress":               "",
		"WebSocketPortNumber":      0,
		"SupportsHttps":            true,
	})
}

func (s *server) jellyfinBranding(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"LoginDisclaimer":     nil,
		"CustomCss":           nil,
		"SplashscreenEnabled": false,
	})
}

// registerJellyfinRoutes exposes the same compatibility surface both under the
// historical /jellyfin-api prefix and at the HTTP root. The official Android
// TV client treats the user-supplied URL as a base URL, so root aliases allow
// https://host itself to work while keeping /api/v1 untouched.
func (s *server) registerJellyfinRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	route := func(path string) string { return p + path }

	mux.HandleFunc("GET "+route("/System/Info/Public"), s.jellyfinCompatInfo)
	mux.HandleFunc("GET "+route("/Users/Public"), s.jellyfinPublicUsers)
	mux.HandleFunc("GET "+route("/Branding/Configuration"), s.jellyfinBranding)
	mux.HandleFunc("POST "+route("/Users/AuthenticateByName"), s.jellyfinAuthenticate)
	mux.HandleFunc("GET "+route("/System/Info"), s.jellyfinRequireAuth(s.jellyfinCompatSystemInfo))
	mux.HandleFunc("POST "+route("/Sessions/Logout"), s.jellyfinRequireAuth(s.jellyfinLogout))
	mux.HandleFunc("GET "+route("/Users/{id}"), s.jellyfinRequireAuth(s.jellyfinCurrentUser))
	mux.HandleFunc("GET "+route("/Users/{id}/Views"), s.jellyfinRequireAuth(s.jellyfinViews))
	mux.HandleFunc("GET "+route("/Library/MediaFolders"), s.jellyfinRequireAuth(s.jellyfinViews))
	mux.HandleFunc("GET "+route("/Users/{id}/Items"), s.jellyfinRequireAuth(s.jellyfinItems))
	mux.HandleFunc("GET "+route("/Users/{id}/Items/Resume"), s.jellyfinRequireAuth(s.jellyfinResume))
	mux.HandleFunc("GET "+route("/Items/{id}"), s.jellyfinRequireAuth(s.jellyfinItem))
	mux.HandleFunc("GET "+route("/Items/{id}/Images/{kind}"), s.jellyfinRequireAuth(s.jellyfinImage))
	mux.HandleFunc("GET "+route("/Items/{id}/PlaybackInfo"), s.jellyfinRequireAuth(s.jellyfinPlaybackInfo))
	mux.HandleFunc("POST "+route("/Items/{id}/PlaybackInfo"), s.jellyfinRequireAuth(s.jellyfinPlaybackInfo))
	mux.HandleFunc("GET "+route("/Shows/{id}/Seasons"), s.jellyfinRequireAuth(s.jellyfinSeasons))
	mux.HandleFunc("GET "+route("/Shows/{id}/Episodes"), s.jellyfinRequireAuth(s.jellyfinEpisodes))
	mux.HandleFunc("GET "+route("/Videos/{id}/stream"), s.jellyfinRequireAuth(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Audio/{id}/stream"), s.jellyfinRequireAuth(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Videos/{id}/{subtitle_id}/Subtitles/{index}/Stream.vtt"), s.jellyfinRequireAuth(s.jellyfinSubtitle))
	mux.HandleFunc("POST "+route("/Sessions/Playing"), s.jellyfinRequireAuth(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Progress"), s.jellyfinRequireAuth(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Stopped"), s.jellyfinRequireAuth(s.jellyfinStopped))
}

type jellyfinStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *jellyfinStatusRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *jellyfinStatusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func isJellyfinRequestPath(path string) bool {
	if strings.HasPrefix(path, "/jellyfin-api/") {
		return true
	}
	for _, prefix := range []string{"/System/", "/Users/", "/Branding/", "/Library/", "/Items/", "/Shows/", "/Videos/", "/Audio/", "/Sessions/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func safeJellyfinQuery(values url.Values) string {
	copyValues := url.Values{}
	for key, list := range values {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "password") {
			copyValues.Set(key, "[redacted]")
			continue
		}
		for _, value := range list {
			copyValues.Add(key, value)
		}
	}
	return copyValues.Encode()
}

func (s *server) jellyfinTrace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isJellyfinRequestPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		recorder := &jellyfinStatusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		detail := fmt.Sprintf(
			"method=%s path=%s query=%q status=%d ua=%q accept=%q content_type=%q auth_present=%t elapsed_ms=%d",
			r.Method,
			r.URL.Path,
			safeJellyfinQuery(r.URL.Query()),
			status,
			shortDevice(r.UserAgent()),
			r.Header.Get("Accept"),
			r.Header.Get("Content-Type"),
			jellyfinToken(r) != "",
			time.Since(started).Milliseconds(),
		)
		s.admin.Log(r.Context(), "info", "jellyfin", "JELLYFIN_REQUEST", nil, detail)
	})
}
