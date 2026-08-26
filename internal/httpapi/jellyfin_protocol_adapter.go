package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// jellyfinTVAuthenticate returns the fields that are required by the Jellyfin
// SDK AuthenticationResult/UserDto/SessionInfoDto models. Internal StormFlix
// integer/string ids are converted to stable Jellyfin UUIDs by
// jellyfinCompatWrap before the response leaves the server.
func (s *server) jellyfinTVAuthenticate(w http.ResponseWriter, r *http.Request) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ResponseStatus": map[string]any{"ErrorCode": "BadRequest", "Message": "Invalid login body"}})
		return
	}
	username := firstJellyfinString(raw, "Username", "username")
	password := firstJellyfinString(raw, "Pw", "Password", "pw", "password")
	u, token, err := s.auth.Login(r.Context(), username, password, shortDevice(r.UserAgent()), clientIP(r))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ResponseStatus": map[string]any{"ErrorCode": "InvalidUser", "Message": "Invalid username or password"}})
		return
	}
	if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
		u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	_ = s.jellyfinDefaultProfileID(r.Context(), u)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionHash := sha256.Sum256([]byte("session|" + token))
	sessionID := hex.EncodeToString(sessionHash[:16])
	user := s.jellyfinUserObject(u)
	// The compatibility surface does not need Jellyfin's very large policy and
	// preference DTOs. They are nullable in UserDto. Returning partial objects
	// makes strict Kotlin serialization fail because those DTOs contain many
	// required fields.
	user["Configuration"] = nil
	user["Policy"] = nil

	writeJSON(w, http.StatusOK, map[string]any{
		"User": user,
		"SessionInfo": map[string]any{
			"PlayableMediaTypes":   []string{"Video", "Audio"},
			"Id":                   sessionID,
			"UserId":               firstJellyfinString(map[string]any{"id": u.ID}, "id"),
			"UserName":             u.Username,
			"Client":               "Jellyfin Android TV",
			"LastActivityDate":     now,
			"LastPlaybackCheckIn":  now,
			"DeviceName":           shortDevice(r.UserAgent()),
			"DeviceType":           "TV",
			"DeviceId":             shortDevice(r.UserAgent()),
			"ApplicationVersion":   jellyfinCompatibilityVersion,
			"IsActive":             true,
			"SupportsMediaControl": false,
			"SupportsRemoteControl": false,
			"HasCustomDeviceName":  false,
			"SupportedCommands":    []string{},
			"ServerId":             s.jellyfinServerID(),
		},
		"AccessToken": token,
		"ServerId":    s.jellyfinServerID(),
	})
}

func (s *server) ensureJellyfinIDMap(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS jellyfin_id_map (
		uuid TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		original TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(kind, original)
	)`)
	return err
}

func isJellyfinUUID(value string) bool {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) != 32 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func (s *server) jellyfinExternalID(ctx context.Context, kind, original string) string {
	original = strings.TrimSpace(original)
	if original == "" || isJellyfinUUID(original) {
		return original
	}
	_ = s.ensureJellyfinIDMap(ctx)
	sum := sha256.Sum256([]byte(s.jellyfinServerID() + "|" + kind + "|" + original))
	id := hex.EncodeToString(sum[:16])
	_, _ = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO jellyfin_id_map(uuid,kind,original) VALUES(?,?,?)`, id, kind, original)
	return id
}

func (s *server) jellyfinInternalID(ctx context.Context, external string) (string, bool) {
	external = strings.ReplaceAll(strings.TrimSpace(external), "-", "")
	if !isJellyfinUUID(external) {
		return external, false
	}
	_ = s.ensureJellyfinIDMap(ctx)
	var original string
	if err := s.db.QueryRowContext(ctx, `SELECT original FROM jellyfin_id_map WHERE uuid=?`, external).Scan(&original); err != nil {
		return external, false
	}
	return original, true
}

func jellyfinIDKind(key string, object map[string]any, original string) string {
	lower := strings.ToLower(original)
	if strings.HasPrefix(lower, "lib") {
		return "library"
	}
	if strings.HasPrefix(lower, "m") {
		return "media"
	}
	if strings.HasPrefix(lower, "series-") {
		return "series"
	}
	if strings.HasPrefix(lower, "season-") {
		return "season"
	}
	if strings.EqualFold(key, "UserId") {
		return "user"
	}
	if key == "Id" {
		if _, ok := object["HasPassword"]; ok {
			return "user"
		}
		typeName := strings.ToLower(strings.TrimSpace(firstJellyfinString(object, "Type")))
		switch typeName {
		case "collectionfolder":
			return "library"
		case "series":
			return "series"
		case "season":
			return "season"
		case "movie", "episode", "audio", "video":
			return "media"
		}
	}
	if strings.EqualFold(key, "SeriesId") {
		return "series"
	}
	if strings.EqualFold(key, "SeasonId") {
		return "season"
	}
	return "generic"
}

func shouldJellyfinTransformID(key string, object map[string]any, value string) bool {
	if value == "" || isJellyfinUUID(value) {
		return false
	}
	switch key {
	case "ServerId", "DeviceId", "PlaylistItemId":
		return false
	}
	if key == "Id" {
		// SessionInfo.Id and MediaSourceInfo.Id are strings in the Jellyfin SDK.
		// User/BaseItem IDs are UUIDs and can be recognized from their object.
		if _, ok := object["UserId"]; ok {
			return false
		}
		if _, ok := object["HasPassword"]; ok {
			return true
		}
		_, hasType := object["Type"]
		return hasType
	}
	return strings.HasSuffix(strings.ToLower(key), "id")
}

func (s *server) transformJellyfinResponse(ctx context.Context, value any) any {
	switch current := value.(type) {
	case []any:
		for i := range current {
			current[i] = s.transformJellyfinResponse(ctx, current[i])
		}
		return current
	case map[string]any:
		if _, isUser := current["HasPassword"]; isUser {
			// Avoid sending incomplete strict DTOs.
			current["Configuration"] = nil
			current["Policy"] = nil
		}
		for key, child := range current {
			switch typed := child.(type) {
			case string:
				if shouldJellyfinTransformID(key, current, typed) {
					current[key] = s.jellyfinExternalID(ctx, jellyfinIDKind(key, current, typed), typed)
				}
			default:
				current[key] = s.transformJellyfinResponse(ctx, child)
			}
		}
		return current
	default:
		return value
	}
}

func (s *server) decodeJellyfinValue(ctx context.Context, value any, key string) any {
	switch current := value.(type) {
	case []any:
		for i := range current {
			current[i] = s.decodeJellyfinValue(ctx, current[i], key)
		}
		return current
	case map[string]any:
		for childKey, child := range current {
			current[childKey] = s.decodeJellyfinValue(ctx, child, childKey)
		}
		return current
	case string:
		if key == "ServerId" || key == "DeviceId" {
			return current
		}
		if original, ok := s.jellyfinInternalID(ctx, current); ok {
			return original
		}
		return current
	default:
		return value
	}
}

func (s *server) decodeJellyfinRequestIDs(r *http.Request) {
	for _, key := range []string{"id", "subtitle_id"} {
		if current := r.PathValue(key); current != "" {
			if original, ok := s.jellyfinInternalID(r.Context(), current); ok {
				r.SetPathValue(key, original)
			}
		}
	}
	query := r.URL.Query()
	changed := false
	for key, values := range query {
		if !strings.Contains(strings.ToLower(key), "id") {
			continue
		}
		for i, value := range values {
			if original, ok := s.jellyfinInternalID(r.Context(), value); ok {
				values[i] = original
				changed = true
			}
		}
		query[key] = values
	}
	if changed {
		r.URL.RawQuery = query.Encode()
	}

	if r.Body == nil || !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		r.Body = io.NopCloser(bytes.NewReader(body))
		return
	}
	var payload any
	if json.Unmarshal(body, &payload) == nil {
		payload = s.decodeJellyfinValue(r.Context(), payload, "")
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			body = encoded
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
}

func isJellyfinBinaryResponse(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/images/") || strings.Contains(lower, "/subtitles/") || strings.HasSuffix(lower, "/stream") || strings.Contains(lower, "/stream?")
}

// jellyfinCompatWrap translates between StormFlix ids and UUIDs without
// changing any /api/v1 model or database primary key. Binary streaming routes
// are only request-decoded; JSON routes are decoded and response-transformed.
func (s *server) jellyfinCompatWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.decodeJellyfinRequestIDs(r)
		if isJellyfinBinaryResponse(r.URL.Path) {
			next(w, r)
			return
		}

		recorder := httptest.NewRecorder()
		next(recorder, r)
		for key, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		status := recorder.Code
		if status == 0 {
			status = http.StatusOK
		}
		body := recorder.Body.Bytes()
		if len(bytes.TrimSpace(body)) > 0 && strings.Contains(strings.ToLower(recorder.Header().Get("Content-Type")), "json") {
			var payload any
			if json.Unmarshal(body, &payload) == nil {
				payload = s.transformJellyfinResponse(r.Context(), payload)
				if encoded, err := json.Marshal(payload); err == nil {
					body = encoded
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
				}
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}
