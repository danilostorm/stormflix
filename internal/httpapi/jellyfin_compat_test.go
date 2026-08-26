package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/danilostorm/stormflix/internal/config"
)

func TestJellyfinPublicInfoMatchesOfficialDiscoveryContract(t *testing.T) {
	s := &server{config: config.Config{DataDir: t.TempDir(), ServerName: "StormFlix Test"}}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "/jellyfin-api")
	s.registerJellyfinRoutes(mux, "")

	for _, path := range []string{"/System/Info/Public", "/jellyfin-api/System/Info/Public"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got := body["ProductName"]; got != "Jellyfin Server" {
				t.Fatalf("ProductName=%v", got)
			}
			version, _ := body["Version"].(string)
			if !regexp.MustCompile(`^\d+\.\d+\.\d+(?:\.\d+)?$`).MatchString(version) {
				t.Fatalf("Version %q is not parseable by Jellyfin clients", version)
			}
			id, _ := body["Id"].(string)
			if !regexp.MustCompile(`^[0-9a-fA-F]{32}$|^[0-9a-fA-F-]{36}$`).MatchString(id) {
				t.Fatalf("Id %q is not UUID compatible", id)
			}
			if completed, _ := body["StartupWizardCompleted"].(bool); !completed {
				t.Fatalf("StartupWizardCompleted=%v", body["StartupWizardCompleted"])
			}
		})
	}
}

func TestJellyfinPingIsJSONString(t *testing.T) {
	s := &server{config: config.Config{DataDir: t.TempDir(), ServerName: "StormFlix Test"}}
	mux := http.NewServeMux()
	s.registerJellyfinRoutes(mux, "")

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/System/Ping", nil)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("content-type=%q", got)
			}
			var name string
			if err := json.Unmarshal(res.Body.Bytes(), &name); err != nil {
				t.Fatalf("ping must be a JSON string: %v body=%q", err, res.Body.String())
			}
			if name != "StormFlix Test" {
				t.Fatalf("ping=%q", name)
			}
		})
	}
}
