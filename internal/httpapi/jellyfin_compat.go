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
	writeJSON(w, http.StatusOK, map[string]any{"IsLocal": false, "IsInNetwork": false})
}

// registerJellyfinRoutes exposes the compatibility facade both under the
// historical /jellyfin-api prefix and at the root paths used by official
// clients. The matrix below is intentionally based on the current official
// Jellyfin Android WebView and Android TV/Fire TV client source. Optional
// features that StormFlix does not implement return strict empty DTO shapes;
// playback/catalog state always comes from native StormFlix services.
func (s *server) registerJellyfinRoutes(mux *http.ServeMux, prefix string) {
	p := strings.TrimSuffix(prefix, "/")
	route := func(path string) string { return p + path }
	compat := func(h http.HandlerFunc) http.HandlerFunc { return s.jellyfinCompatWrap(h) }
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return s.jellyfinCompatWrap(s.jellyfinRequireAuth(s.jellyfinNormalizeLibraryScope(h)))
	}

	// Register native WebView bootstrap and native Playback Anywhere DLNA APIs
	// only once. The web path performs SSDP from the StormFlix host; Android has
	// an additional device-local SSDP implementation for the phone/TV LAN.
	if p != "" {
		mux.HandleFunc("GET /api/v1/compat/jellyfin-mobile-bridge", s.requireAuth(s.jellyfinMobileBridge))
		mux.HandleFunc("GET /api/v1/playback/dlna/devices", s.requireAuth(s.dlnaDevices))
		mux.HandleFunc("POST /api/v1/playback/dlna/{device_id}/play", s.requireAuth(s.dlnaPlay))
		mux.HandleFunc("POST /api/v1/playback/dlna/{device_id}/control", s.requireAuth(s.dlnaControl))
	}

	// Discovery / connection / authentication.
	mux.HandleFunc("GET "+route("/System/Info/Public"), compat(s.jellyfinCompatInfo))
	mux.HandleFunc("GET "+route("/System/Ping"), s.jellyfinPingJSON)
	mux.HandleFunc("POST "+route("/System/Ping"), s.jellyfinPingJSON)
	mux.HandleFunc("GET "+route("/Users/Public"), compat(s.jellyfinPublicUsers))
	mux.HandleFunc("GET "+route("/Branding/Configuration"), compat(s.jellyfinBranding))
	mux.HandleFunc("GET "+route("/Startup/Configuration"), compat(s.jellyfinPublicConfiguration))
	mux.HandleFunc("GET "+route("/QuickConnect/Enabled"), compat(s.jellyfinQuickConnectEnabled))
	mux.HandleFunc("POST "+route("/Users/AuthenticateByName"), compat(s.jellyfinTVAuthenticateMinimal))

	// Critical Android/Android TV post-login startup sequence.
	mux.HandleFunc("GET "+route("/System/Info"), authed(s.jellyfinCompatSystemInfo))
	mux.HandleFunc("GET "+route("/System/Endpoint"), authed(s.jellyfinEndpoint))
	mux.HandleFunc("GET "+route("/Users/Me"), authed(s.jellyfinMe))
	mux.HandleFunc("GET "+route("/DisplayPreferences/{id}"), authed(s.jellyfinDisplayPreferences))
	mux.HandleFunc("POST "+route("/Sessions/Capabilities"), authed(s.jellyfinCapabilities))
	mux.HandleFunc("POST "+route("/Sessions/Capabilities/Full"), authed(s.jellyfinCapabilities))
	mux.HandleFunc("POST "+route("/Sessions/Logout"), authed(s.jellyfinLogout))
	mux.HandleFunc("GET "+route("/Sessions"), authed(s.jellyfinSessions))
	mux.HandleFunc("POST "+route("/Sessions/{session_id}/Playing"), authed(s.jellyfinDLNAPlay))
	mux.HandleFunc("POST "+route("/Sessions/{session_id}/Playing/{command}"), authed(s.jellyfinDLNAControl))

	// Home/catalog routes used by modern Android TV and SDK clients.
	mux.HandleFunc("GET "+route("/UserViews"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/UserViews/GroupingOptions"), authed(s.jellyfinUserViewGroupingOptions))
	mux.HandleFunc("GET "+route("/UserItems/Resume"), authed(s.jellyfinResume))
	mux.HandleFunc("GET "+route("/Items/Latest"), authed(s.jellyfinLatestCatalog))
	mux.HandleFunc("GET "+route("/Items"), authed(s.jellyfinCatalogItems))
	mux.HandleFunc("GET "+route("/Genres"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Persons"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("POST "+route("/ClientLog/Document"), authed(s.jellyfinClientLogDocument))

	mux.HandleFunc("GET "+route("/Users/{id}"), authed(s.jellyfinCurrentUser))
	mux.HandleFunc("GET "+route("/Users/{id}/Views"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/Library/MediaFolders"), authed(s.jellyfinRichViews))
	mux.HandleFunc("GET "+route("/Users/{id}/Items"), authed(s.jellyfinCatalogItems))
	mux.HandleFunc("GET "+route("/Users/{id}/Items/Resume"), authed(s.jellyfinResume))
	mux.HandleFunc("GET "+route("/Users/{id}/Items/Latest"), authed(s.jellyfinLatestCatalog))
	mux.HandleFunc("GET "+route("/Users/{id}/Suggestions"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Shows/NextUp"), authed(s.jellyfinNextUp))
	mux.HandleFunc("GET "+route("/Shows/Upcoming"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Items/{id}"), authed(s.jellyfinCatalogItem))
	mux.HandleFunc("GET "+route("/Items/{id}/Similar"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Items/{id}/ThemeSongs"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Items/{id}/ThemeVideos"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Items/{id}/SpecialFeatures"), authed(s.jellyfinEmptyItems))
	mux.HandleFunc("GET "+route("/Items/{id}/Intros"), authed(s.jellyfinEmptyItems))

	// Android TV's image pipeline intentionally performs these GETs without the
	// session token. Only read-only artwork is public.
	mux.HandleFunc("GET "+route("/Items/{id}/Images/{kind}"), compat(s.jellyfinPublicCatalogImage))
	mux.HandleFunc("GET "+route("/Items/{id}/PlaybackInfo"), authed(s.jellyfinPlaybackInfo))
	mux.HandleFunc("POST "+route("/Items/{id}/PlaybackInfo"), authed(s.jellyfinPlaybackInfo))
	mux.HandleFunc("GET "+route("/Shows/{id}/Seasons"), authed(s.jellyfinSeasons))
	mux.HandleFunc("GET "+route("/Shows/{id}/Episodes"), authed(s.jellyfinEpisodes))
	mux.HandleFunc("GET "+route("/Videos/{id}/stream"), authed(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Audio/{id}/stream"), authed(s.jellyfinStream))
	mux.HandleFunc("GET "+route("/Videos/{id}/{subtitle_id}/Subtitles/{index}/Stream.vtt"), authed(s.jellyfinSubtitle))
	mux.HandleFunc("GET "+route("/Playback/BitrateTest"), authed(s.jellyfinPlaybackBitrateTest))
	mux.HandleFunc("POST "+route("/Sessions/Playing"), authed(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Progress"), authed(s.jellyfinProgress))
	mux.HandleFunc("POST "+route("/Sessions/Playing/Stopped"), authed(s.jellyfinStopped))
}

type jellyfinStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *jellyfinStatusRecorder) WriteHeader(status int) {
	if w.status == 0 { w.status = status }
	w.ResponseWriter.WriteHeader(status)
}
func (w *jellyfinStatusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 { w.status = http.StatusOK }
	return w.ResponseWriter.Write(data)
}

func isJellyfinRequestPath(path string) bool {
	if strings.HasPrefix(path, "/jellyfin-api/") { return true }
	for _, prefix := range []string{
		"/System/", "/Users/", "/UserViews", "/UserItems/", "/Branding/", "/Startup/", "/QuickConnect/",
		"/DisplayPreferences/", "/Library/", "/Items", "/Shows/", "/Genres", "/Persons", "/Playback/",
		"/Videos/", "/Audio/", "/Sessions/", "/ClientLog/",
	} {
		if strings.HasPrefix(path, prefix) { return true }
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
		for _, value := range list { copyValues.Add(key, value) }
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
		if status == 0 { status = http.StatusOK }
		detail := fmt.Sprintf(
			"method=%s path=%s query=%q status=%d host=%q forwarded_host=%q forwarded_proto=%q ua=%q accept=%q content_type=%q auth_present=%t elapsed_ms=%d",
			r.Method, r.URL.Path, safeJellyfinQuery(r.URL.Query()), status, r.Host,
			r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"), shortDevice(r.UserAgent()),
			r.Header.Get("Accept"), r.Header.Get("Content-Type"), jellyfinToken(r) != "", time.Since(started).Milliseconds(),
		)
		level := "info"
		message := "JELLYFIN_REQUEST"
		if status == http.StatusNotFound || status >= 500 {
			level = "warn"
			message = "JELLYFIN_COMPAT_GAP"
		}
		s.admin.Log(r.Context(), level, "jellyfin", message, nil, detail)
	})
}
