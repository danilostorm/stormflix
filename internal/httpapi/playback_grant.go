package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danilostorm/stormflix/internal/auth"
)

const playbackGrantTTL = 12 * time.Hour

var playbackGrantKeyMu sync.Mutex

type playbackGrantClaims struct {
	Version   int    `json:"v"`
	UserID    int64  `json:"uid"`
	ProfileID int64  `json:"pid,omitempty"`
	MediaID   int64  `json:"mid"`
	Expires   int64  `json:"exp"`
	Nonce     string `json:"n"`
}

type playbackGrantInput struct {
	URL string `json:"url"`
}

type playbackGrantOutput struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *server) playbackGrantKey() ([]byte, error) {
	playbackGrantKeyMu.Lock()
	defer playbackGrantKeyMu.Unlock()
	path := filepath.Join(s.config.DataDir, "playback-grants.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid playback grant key")
	}
	return key, nil
}

func (s *server) issuePlaybackGrant(userID, profileID, mediaID int64) (string, time.Time, error) {
	if userID <= 0 || mediaID <= 0 {
		return "", time.Time{}, errors.New("invalid playback grant identity")
	}
	key, err := s.playbackGrantKey()
	if err != nil {
		return "", time.Time{}, err
	}
	nonceBytes := make([]byte, 12)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(playbackGrantTTL)
	claims := playbackGrantClaims{Version: 1, UserID: userID, ProfileID: profileID, MediaID: mediaID, Expires: expires.Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonceBytes)}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, expires, nil
}

func (s *server) parsePlaybackGrant(raw string) (playbackGrantClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return playbackGrantClaims{}, errors.New("invalid playback grant")
	}
	key, err := s.playbackGrantKey()
	if err != nil {
		return playbackGrantClaims{}, err
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return playbackGrantClaims{}, errors.New("invalid playback grant signature")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	if len(provided) != len(want) || subtle.ConstantTimeCompare(provided, want) != 1 {
		return playbackGrantClaims{}, errors.New("invalid playback grant signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return playbackGrantClaims{}, errors.New("invalid playback grant payload")
	}
	var claims playbackGrantClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return playbackGrantClaims{}, errors.New("invalid playback grant payload")
	}
	if claims.Version != 1 || claims.UserID <= 0 || claims.MediaID <= 0 || claims.Expires <= time.Now().UTC().Unix() || claims.Nonce == "" {
		return playbackGrantClaims{}, errors.New("playback grant expired or invalid")
	}
	return claims, nil
}

func playbackGrantAllowedPath(mediaID int64, path string) bool {
	base := fmt.Sprintf("/api/v1/media/%d/", mediaID)
	if !strings.HasPrefix(path, base) {
		return false
	}
	rest := strings.TrimPrefix(path, base)
	return rest == "stream" || rest == "remux" || strings.HasPrefix(rest, "hls/") || strings.HasPrefix(rest, "webstream/") || strings.HasPrefix(rest, "subtitles/")
}

func playbackGrantMediaID(path string) int64 {
	const prefix = "/api/v1/media/"
	if !strings.HasPrefix(path, prefix) {
		return 0
	}
	rest := strings.TrimPrefix(path, prefix)
	part, _, _ := strings.Cut(rest, "/")
	id, _ := strconv.ParseInt(part, 10, 64)
	return id
}

func (s *server) playbackGrantUser(r *http.Request) (auth.User, int64, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return auth.User{}, 0, errors.New("playback grant only supports media reads")
	}
	raw := strings.TrimSpace(r.URL.Query().Get("st"))
	if raw == "" {
		return auth.User{}, 0, errors.New("playback grant missing")
	}
	claims, err := s.parsePlaybackGrant(raw)
	if err != nil {
		return auth.User{}, 0, err
	}
	if claims.MediaID != playbackGrantMediaID(r.URL.Path) || !playbackGrantAllowedPath(claims.MediaID, r.URL.Path) {
		return auth.User{}, 0, errors.New("playback grant scope mismatch")
	}
	u, err := s.auth.GetUser(r.Context(), claims.UserID)
	if err != nil || !u.Active {
		return auth.User{}, 0, errors.New("playback grant user is unavailable")
	}
	if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
		u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
	}
	return u, claims.ProfileID, nil
}

func (s *server) createPlaybackGrant(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	if !s.requireKidsMediaAccess(w, r, u.ID, id) {
		return
	}
	if _, ok := s.authorizeHLSMedia(w, r, id); !ok {
		return
	}
	var in playbackGrantInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	parsed, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || parsed.Path == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid playback URL"))
		return
	}
	if parsed.IsAbs() && !strings.EqualFold(parsed.Host, r.Host) {
		writeError(w, http.StatusBadRequest, errors.New("playback URL must belong to this StormFlix server"))
		return
	}
	if !playbackGrantAllowedPath(id, parsed.Path) {
		writeError(w, http.StatusBadRequest, errors.New("playback URL is outside the signed media scope"))
		return
	}
	profileID := s.selectedProfileID(r, u.ID)
	token, expires, err := s.issuePlaybackGrant(u.ID, profileID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	query := parsed.Query()
	query.Del("st")
	query.Set("st", token)
	parsed.RawQuery = query.Encode()
	// Return an absolute URL so Chromecast, TV apps and external players can
	// resolve it without inheriting browser cookies or page origin state.
	if !parsed.IsAbs() {
		scheme := "http"
		if isHTTPS(r) {
			scheme = "https"
		}
		parsed.Scheme = scheme
		parsed.Host = r.Host
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, playbackGrantOutput{URL: parsed.String(), ExpiresAt: expires.Format(time.RFC3339)})
}

func appendPlaybackGrant(rawURL, token string) string {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(token) == "" {
		return rawURL
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "data" {
		return rawURL
	}
	query := parsed.Query()
	query.Set("st", token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func rewritePlaylistWithPlaybackGrant(playlist, token string) string {
	if strings.TrimSpace(token) == "" || playlist == "" {
		return playlist
	}
	lines := strings.Split(playlist, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			lines[i] = appendPlaybackGrant(trimmed, token)
			continue
		}
		// HLS init maps and other URI-bearing tags keep their syntax while the
		// referenced resource receives the same temporary grant.
		const marker = `URI="`
		start := strings.Index(line, marker)
		if start < 0 {
			continue
		}
		start += len(marker)
		end := strings.Index(line[start:], `"`)
		if end < 0 {
			continue
		}
		end += start
		lines[i] = line[:start] + appendPlaybackGrant(line[start:end], token) + line[end:]
	}
	return strings.Join(lines, "\n")
}
