package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danilostorm/stormflix/internal/config"
)

func TestJellyfinAndroidDiscoveryUsesForwardedPublicAddress(t *testing.T) {
	s := &server{config: config.Config{DataDir: t.TempDir(), ServerName: "StormFlix Test"}}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "")

	req := httptest.NewRequest(http.MethodGet, "http://internal/System/Info/Public", nil)
	req.Host = "stormflix.internal:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "stormflix.example")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["LocalAddress"]; got != "https://stormflix.example" {
		t.Fatalf("LocalAddress=%v", got)
	}
	if got := body["ProductName"]; got != "Jellyfin Server" {
		t.Fatalf("ProductName=%v", got)
	}
}

func TestJellyfinSystemPingMatchesOfficialSurface(t *testing.T) {
	s := &server{config: config.Config{DataDir: t.TempDir(), ServerName: "StormFlix Test"}}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "")

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/System/Ping", nil)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", method, res.Code, res.Body.String())
		}
		if strings.TrimSpace(res.Body.String()) != "StormFlix Test" {
			t.Fatalf("%s body=%q", method, res.Body.String())
		}
	}
}
