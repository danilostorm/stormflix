package httpapi

import (
	"errors"
	"net/http"

	"github.com/danilostorm/stormflix/internal/transcode"
)

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	engine := transcode.Detect()
	writeJSON(w, 200, map[string]any{"status": "ok", "transcoding": engine.FFmpegPath != "", "version": version})
}
func (s *server) systemInfo(w http.ResponseWriter, r *http.Request) {
	engine := transcode.Detect()
	writeJSON(w, 200, map[string]any{
		"name": "StormFlix", "version": version,
		"playback_engine": "v6", "direct_play_first": true, "direct_play_only": false,
		"transcoding_enabled": engine.FFmpegPath != "", "supported_extensions": s.librariesExtensions(),
	})
}
func (s *server) librariesExtensions() []string {
	return []string{".avi", ".m2ts", ".m4v", ".mkv", ".mov", ".mp4", ".mpeg", ".mpg", ".ts", ".webm"}
}
func (s *server) setupStatus(w http.ResponseWriter, r *http.Request) {
	v, err := s.auth.NeedsSetup(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"needs_setup": v})
}
func (s *server) createFirstAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	u, err := s.auth.CreateFirstAdmin(r.Context(), in.Username, in.DisplayName, in.Password)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	id := u.ID
	s.admin.Log(r.Context(), "info", "auth", "Initial administrator created", &id, u.Username)
	writeJSON(w, 201, u)
}
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	u, token, err := s.auth.Login(r.Context(), in.Username, in.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		writeError(w, 401, err)
		return
	}
	clearProfileCookie(w)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r), MaxAge: 30 * 24 * 3600})
	id := u.ID
	s.admin.Log(r.Context(), "info", "auth", "Login successful", &id, clientIP(r))
	writeJSON(w, 200, u)
}
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	clearProfileCookie(w)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u.ID == 0 {
		writeError(w, 401, errors.New("authentication required"))
		return
	}
	writeJSON(w, 200, u)
}
