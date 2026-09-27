package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/playback"
)

func TestOriginalPlaybackDoesNotProbeRemoteFileAndIncludesProfileState(t *testing.T) {
	s, _, _ := versionSelectionServer(t)
	user, err := s.auth.CreateFirstAdmin(context.Background(), "admin", "Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.auth.CreateProfile(context.Background(), user.ID, "Test", "storm-red", "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`DELETE FROM media_technical`,
		`INSERT INTO subtitles(media_id,language,format) VALUES(10,'pt','srt')`,
	} {
		if _, err := s.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO profile_progress(profile_id,media_id,position_seconds) VALUES(?,10,123)`, profile.ID); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media/10/playback/original", nil)
	r.SetPathValue("id", "10")
	r.AddCookie(&http.Cookie{Name: profileCookie, Value: strconv.FormatInt(profile.ID, 10)})
	r = r.WithContext(context.WithValue(r.Context(), userKey, user))
	w := httptest.NewRecorder()
	s.originalPlayback(w, r)
	var body struct {
		playback.Plan
		Preferences playbackPreferenceState `json:"playback_preferences"`
		Subtitles   []struct {
			Language string `json:"language"`
		} `json:"subtitles"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatalf("response %d: %s", w.Code, w.Body.String())
	}
	if !body.Available || !body.OriginalOnly || body.Mode != playback.ModeDirectPlay || body.URL != "/api/v1/media/10/stream" || body.ResumePositionSeconds != 123 || body.PlaybackSessionID == "" {
		t.Fatalf("unexpected original plan: %+v", body)
	}
	if body.Preferences.RewindSeconds != 10 || len(body.Subtitles) != 1 || body.Subtitles[0].Language != "pt" {
		t.Fatalf("missing profile/sidecar state: %+v", body)
	}
	var probes int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM media_technical`).Scan(&probes); err != nil || probes != 0 {
		t.Fatalf("fast route probed remote media: %d %v", probes, err)
	}
	// Both the fast route and retired conversion routes still enforce Kids access.
	if _, err := s.db.Exec(`UPDATE profiles SET is_kids=1,content_rating_limit=6 WHERE id=?`, profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE media_metadata SET content_rating_age=18 WHERE media_id=10`); err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.HandlerFunc{s.originalPlayback, s.originalOnlyTransport} {
		w = httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("kids access: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestOriginalRoutesEnforceLibraryAccessAndRetireConversion(t *testing.T) {
	s, _, request := versionSelectionServer(t)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media/10/playback/original", nil)
	r.SetPathValue("id", "10")
	r = r.WithContext(context.WithValue(r.Context(), userKey, auth.User{ID: 1, Role: "user", LibraryIDs: []int64{2}}))
	for _, handler := range []http.HandlerFunc{s.originalPlayback, s.originalOnlyTransport} {
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("library access: %d %s", w.Code, w.Body.String())
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), userKey, auth.User{ID: 1, Role: "admin"}))
	w := httptest.NewRecorder()
	s.originalOnlyTransport(w, r)
	if w.Code != http.StatusConflict || !bytes.Contains(w.Body.Bytes(), []byte("server_conversion_disabled")) {
		t.Fatalf("conversion still exposed: %d %s", w.Code, w.Body.String())
	}
	request.Quality = "480p"
	body, _ := json.Marshal(request)
	r = httptest.NewRequest(http.MethodPost, "/api/v1/media/20/playback/plan", bytes.NewReader(body))
	r.SetPathValue("id", "20")
	r = r.WithContext(context.WithValue(r.Context(), userKey, auth.User{ID: 1, Role: "admin"}))
	w = httptest.NewRecorder()
	s.playbackPlan(w, r)
	var plan playback.Plan
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
	if !plan.Available || !plan.OriginalOnly || plan.Mode != playback.ModeDirectPlay || plan.Quality != "original" || plan.URL != "/api/v1/media/20/stream" {
		t.Fatalf("old client started conversion: %+v", plan)
	}
}
