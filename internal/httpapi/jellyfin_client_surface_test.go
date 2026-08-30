package httpapi

import (
	"net/http"
	"testing"
)

func TestOfficialJellyfinClientRouteMatrixRegisters(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "/jellyfin-api")
	s.registerJellyfinRoutes(mux, "")

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/System/Info/Public"},
		{http.MethodGet, "/Startup/Configuration"},
		{http.MethodGet, "/QuickConnect/Enabled"},
		{http.MethodPost, "/Users/AuthenticateByName"},
		{http.MethodGet, "/UserViews"},
		{http.MethodGet, "/Items"},
		{http.MethodGet, "/Items/Latest"},
		{http.MethodGet, "/Items/0123456789abcdef0123456789abcdef/Similar"},
		{http.MethodGet, "/Shows/NextUp"},
		{http.MethodPost, "/Sessions/Capabilities/Full"},
		{http.MethodGet, "/Playback/BitrateTest"},
		{http.MethodGet, "/jellyfin-api/UserViews"},
		{http.MethodGet, "/jellyfin-api/Items"},
		{http.MethodPost, "/jellyfin-api/Sessions/Capabilities/Full"},
		{http.MethodGet, "/api/v1/compat/jellyfin-mobile-bridge"},
	} {
		req, err := http.NewRequest(tc.method, "http://stormflix.local"+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Fatalf("missing %s %s", tc.method, tc.path)
		}
	}
}
