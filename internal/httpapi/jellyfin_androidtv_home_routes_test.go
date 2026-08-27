package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJellyfinAndroidTVModernHomeRoutesRegistered(t *testing.T) {
	s := &server{}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "")

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/UserViews"},
		{http.MethodGet, "/UserViews/GroupingOptions"},
		{http.MethodGet, "/UserItems/Resume"},
		{http.MethodGet, "/Items/Latest"},
		{http.MethodGet, "/Items"},
		{http.MethodPost, "/ClientLog/Document"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			_, pattern := mux.Handler(req)
			if pattern == "" {
				t.Fatalf("route not registered: %s %s", tc.method, tc.path)
			}
			if !isJellyfinRequestPath(tc.path) {
				t.Fatalf("route is not included in Jellyfin tracing: %s", tc.path)
			}
		})
	}
}
