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

func jellyfinRequestBaseURL(r *http.Request) string {
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return strings.ToLower(proto) + "://" + host
}

func (s *server) jellyfinCompatInfo(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"LocalAddress":               jellyfinRequestBaseURL(r),
		"ServerName":                 name,
		"Version":                    jellyfinCompatibilityVersion,
		"ProductName":                "Jellyfin Server",
		"OperatingSystem":            "Linux",
		"OperatingSystemDisplayName": "StormFlix",
		"Id":                         s.jellyfinServerID(),
		"StartupWizardCompleted":     true,
	})
}

func (s *server) jellyfinCompatSystemInfo(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	baseURL := jellyfinRequestBaseURL(r)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"LocalAddress":               baseURL,
		"ServerName":                 name,
		"Version":                    jellyfinCompatibilityVersion,
		"ProductName":                "Jellyfin Server",
		"OperatingSystem":            "Linux",
		"OperatingSystemDisplayName": "StormFlix",
		"Id":                         s.jellyfinServerID(),
		"StartupWizardCompleted":     true,
		"WanAddress":                 baseURL,
		"WebSocketPortNumber":        0,
		"SupportsHttps":              isHTTPS(r),
	})
}

func (s *server) jellyfinBranding(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"LoginDisclaimer":     nil,
		"CustomCss":           nil,
		"SplashscreenEnabled": false,
	})
}

// Jellyfin exposes System/Ping publicly and some clients use it while
// validating or refreshing a saved endpoint.
func (s *server) jellyfinPing(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(name))
}

func (s *server) jellyfinEndpoint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"IsLocal":     false,
		"IsInNetwork": false,
	})
}

// registerJellyfinRoutes exposes the same compatibility surface both under the
// historical /jellyfin-api prefix and at the HTTP root. All protocol-facing
// JSON goes through jellyfinCompatWrap so strict Jellyfin SDK UUID/DTO rules do
// not leak into StormFlix's native integer/string ids.
func (s *server) registerJellyfinRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	route := func(path string) string { return p + path }
	compat := func(h http.HandlerFunc) http.HandlerFunc { return s.jellyfinCompatWrap(h) }
	authed := func(h http.HandlerFunc) http.HandlerFunc { return s.jellyfinCompatWrap(s.jellyfinRequireAuth(h)) }

	mux.HandleFunc("GET "+route("/System/Info/Public"), compat(s.jellyfinCompatInfo))
	mux.HandleFunc("GET "+route("/System/Ping"), s.jellyfinPingJSON)
	mux.HandleFunc("POST "+route("/System/Ping"), s.jellyfinPingJSON)
	mux.HandleFunc("GET "+route("/Users/Public"), compat(s.jellyfinPublicUsers))
	mux.HandleFunc("GET "+route("/Branding/Configuration"), compat(s.jellyfinBranding))
	mux.HandleFunc("POST "+route("/Users/AuthenticateByName"), compat(s.jellyfinTVAuthenticateMinimal))

	// Critical Android/Android TV post-login startup sequence.
	mux.HandleFunc("GET "+route("/System/Info"), authed(s.jellyfinCompatSystemInfo))
	mux.HandleFunc("GET "+route("/System/Endpoint"), authed(s.jellyfinEndpoint))
	mux.HandleFunc("GET "+route("/Users/Me"), authed(s.jellyfinMe))
	mux.HandleFunc("GET "+route("/DisplayPreferences/{id}"), authed(s.jellyfinDisplayPreferences))
	mux.HandleFunc("POST "+route("/Sessions/Capabilities"), authed(s.jellyfinCapabilities))
	mux.HandleFunc("POST "+route("/Sessions/Capabilities/Full"), authed(s.jellyfinCapabilities))
	mux.HandleFunc("POST "+route("/Sessions/Logout"), authed(s.jellyfinLogout))

	// Modern SDK routes used by Jellyfin Android TV 0.19.x while building Home.
	// Keep the older /Users/{id}/... aliases below for legacy clients.
	mux.HandleFunc("GET "+route("/UserViews"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/UserViews/GroupingOptions"), authed(s.jellyfinUserViewGroupingOptions))
	mux.HandleFunc("GET "+route("/UserItems/Resume"), authed(s.jellyfinResume))
	mux.HandleFunc("GET "+route("/Items/Latest"), authed(s.jellyfinLatestCatalog))
	mux.HandleFunc("GET "+route("/Items"), authed(s.jellyfinCatalogItems))
	mux.HandleFunc("POST "+route("/ClientLog/Document"), authed(s.jellyfinClientLogDocument))

	mux.HandleFunc("GET "+route("/Users/{id}"), authed(s.jellyfinCurrentUser))
	mux.HandleFunc("GET "+route("/Users/{id}/Views"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/Library/MediaFolders"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/Users/{id}/Items"), authed(s.jellyfinCatalogItems))
	mux.HandleFunc("GET "+route("/Users/{id}/Items/Resume"), authed(s.jellyfinResume))
	mux.HandleFunc("GET "+route("/Users/{id}/Items/Latest"), authed(s.jellyfinLatestCatalog))
	mux.HandleFunc("GET "+route("/Shows/NextUp"), authed(s.jellyfinNextUp))
	mux.HandleFunc("GET "+route("/Items/{id}"), authed(s.jellyfinCatalogItem))
	mux.HandleFunc("GET "+route("/Items/{id}/Images/{kind}"), authed(s.jellyfinCatalogImage))
	mux.HandleFunc("GET "+route("/Items/{id}/PlaybackInfo"), authed(s.jellyfinPlaybackInfo))
	mux.HandleFunc("POST "+route("/Items/{id}/PlaybackInfo"), authed(s.jellyfinPlaybackInfo))
	mux.HandleFunc("GET "+route("/Shows/{id}/Seasons"), authed(s.jellyfinSeasons))
	mux.HandleFunc("GET "+route("/Shows/{id}/Episodes"), authed(s.jellyfinEpisodes))
	mux.HandleFunc("GET "+route("/Videos/{id}/stream"), authed(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Audio/{id}/stream"), authed(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Videos/{id}/{subtitle_id}/Subtitles/{index}/Stream.vtt"), authed(s.jellyfinSubtitle))
	mux.HandleFunc("POST "+route("/Sessions/Playing"), authed(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Progress"), authed(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Stopped"), authed(s.jellyfinStopped))
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
	for _, prefix := range []string{
		"/System/",
		"/Users/",
		"/UserViews",
		"/UserItems/",
		"/Branding/",
		"/DisplayPreferences/",
		"/Library/",
		"/Items",
		"/Shows/",
		"/Videos/",
		"/Audio/",
		"/Sessions/",
		"/ClientLog/",
	} {
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
			"method=%s path=%s query=%q status=%d host=%q forwarded_host=%q forwarded_proto=%q ua=%q accept=%q content_type=%q auth_present=%t elapsed_ms=%d",
			r.Method,
			r.URL.Path,
			safeJellyfinQuery(r.URL.Query()),
			status,
			r.Host,
			r.Header.Get("X-Forwarded-Host"),
			r.Header.Get("X-Forwarded-Proto"),
			shortDevice(r.UserAgent()),
			r.Header.Get("Accept"),
			r.Header.Get("Content-Type"),
			jellyfinToken(r) != "",
			time.Since(started).Milliseconds(),
		)
		s.admin.Log(r.Context(), "info", "jellyfin", "JELLYFIN_REQUEST", nil, detail)
	})
}
