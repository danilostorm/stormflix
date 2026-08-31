package trakt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danilostorm/stormflix/internal/database"
	appsettings "github.com/danilostorm/stormflix/internal/settings"
)

func TestDeviceOAuthIsStoredEncryptedPerProfile(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "stormflix.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	res, err := db.Exec(`INSERT INTO users(username,display_name,password_hash,role,active) VALUES('tester','Tester','x','user',1)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO profiles(user_id,name,active) VALUES(?,?,1)`, userID, "Sala")
	if err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	profileID, _ := res.LastInsertId()
	secretStore, err := appsettings.New(db, t.TempDir())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}

	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "device-1", "user_code": "ABCD1234", "verification_url": "https://trakt.example/activate", "expires_in": 600, "interval": 5})
		case "/oauth/device/token":
			if polls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-plain", "refresh_token": "refresh-plain", "expires_in": 604800, "created_at": 1788210000})
		case "/users/settings":
			if got := r.Header.Get("Authorization"); got != "Bearer access-plain" {
				t.Errorf("unexpected auth header %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"username": "stormviewer", "ids": map[string]any{"slug": "stormviewer"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	s := New(db, secretStore, "client", "secret", "urn:ietf:wg:oauth:2.0:oob")
	s.authBase = server.URL
	s.apiBase = server.URL
	status, err := s.BeginDevice(t.Context(), profileID)
	if err != nil {
		t.Fatalf("begin device: %v", err)
	}
	if !status.Authorization || status.UserCode != "ABCD1234" {
		t.Fatalf("unexpected device status: %+v", status)
	}
	status, err = s.PollDevice(t.Context(), profileID)
	if err != nil {
		t.Fatalf("pending poll: %v", err)
	}
	if !status.Authorization || status.Connected {
		t.Fatalf("pending poll should stay pending: %+v", status)
	}
	status, err = s.PollDevice(t.Context(), profileID)
	if err != nil {
		t.Fatalf("success poll: %v", err)
	}
	if !status.Connected || status.Username != "stormviewer" {
		t.Fatalf("unexpected connected status: %+v", status)
	}
	var access, refresh string
	if err := db.QueryRow(`SELECT access_token,refresh_token FROM profile_trakt WHERE profile_id=?`, profileID).Scan(&access, &refresh); err != nil {
		t.Fatalf("stored tokens: %v", err)
	}
	if !strings.HasPrefix(access, "enc:v1:") || !strings.HasPrefix(refresh, "enc:v1:") || strings.Contains(access, "access-plain") {
		t.Fatalf("tokens were not encrypted at rest")
	}
}

func TestScrobbleAction(t *testing.T) {
	cases := map[string]string{"playing": "start", "paused": "pause", "ended": "stop", "stopped": "stop"}
	for state, want := range cases {
		if got := scrobbleAction(state, 42); got != want {
			t.Fatalf("state %s: got %s want %s", state, got, want)
		}
	}
	if got := scrobbleAction("playing", 100); got != "stop" {
		t.Fatalf("100%% progress should stop, got %s", got)
	}
}
