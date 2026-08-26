package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// jellyfinTVAuthenticateMinimal intentionally returns only the fields the
// official Android/Android TV clients need from AuthenticationResult. The
// SessionInfo DTO is nullable in the Jellyfin SDK and contains many strict
// fields that vary between SDK generations; omitting it avoids turning a
// successful login into an InvalidContentException on the client.
func (s *server) jellyfinTVAuthenticateMinimal(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ResponseStatus": map[string]any{
				"ErrorCode": "BadRequest",
				"Message":   "Invalid login body",
			},
		})
		return
	}

	username := firstJellyfinString(raw, "Username", "username")
	password := firstJellyfinString(raw, "Pw", "Password", "pw", "password")
	u, token, err := s.auth.Login(r.Context(), username, password, shortDevice(r.UserAgent()), clientIP(r))
	if err != nil {
		s.admin.Log(r.Context(), "warn", "jellyfin", "JELLYFIN_AUTH_FAILED", nil,
			fmt.Sprintf("username=%q reason=%q", strings.TrimSpace(username), err.Error()))
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"ResponseStatus": map[string]any{
				"ErrorCode": "InvalidUser",
				"Message":   "Invalid username or password",
			},
		})
		return
	}

	if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
		u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	_ = s.jellyfinDefaultProfileID(r.Context(), u)

	user := s.jellyfinUserObject(u)
	// UserConfiguration and UserPolicy are nullable. Partial versions of those
	// DTOs are less compatible than null across Jellyfin SDK generations.
	user["Configuration"] = nil
	user["Policy"] = nil

	uid := u.ID
	s.admin.Log(r.Context(), "info", "jellyfin", "JELLYFIN_AUTH_SUCCESS", &uid,
		fmt.Sprintf("username=%q client=%q", u.Username, shortDevice(r.UserAgent())))

	writeJSON(w, http.StatusOK, map[string]any{
		"User":        user,
		"AccessToken": token,
		"ServerId":    s.jellyfinServerID(),
	})
}

// Jellyfin's System/Ping response is a JSON string, not raw text. The Kotlin
// SDK deserializes it as String, so returning unquoted text can become an
// InvalidContentException even though the HTTP status is 200.
func (s *server) jellyfinPingJSON(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(s.config.ServerName)
	if name == "" {
		name = "StormFlix"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, name)
}
