package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHLSRoutePatternRegistersAndMatches(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(hlsRoutePattern, func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("id"); got != "42" {
			t.Fatalf("id=%q", got)
		}
		if got := r.PathValue("session"); got != "abc123" {
			t.Fatalf("session=%q", got)
		}
		if got := r.PathValue("rest"); got != "segment/7.m4s" {
			t.Fatalf("rest=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/42/hls/abc123/segment/7.m4s", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rec.Code)
	}
}
