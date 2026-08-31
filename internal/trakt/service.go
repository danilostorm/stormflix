package trakt

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	appsettings "github.com/danilostorm/stormflix/internal/settings"
)

const (
	defaultAPIBase  = "https://api.trakt.tv"
	defaultAuthBase = "https://auth.trakt.tv"
)

var (
	ErrNotConfigured = errors.New("Trakt is not configured on this server")
	ErrPending       = errors.New("Trakt authorization is still pending")
	ErrExpired       = errors.New("Trakt authorization code expired")
)

type Service struct {
	db           *sql.DB
	secrets      *appsettings.Service
	client       *http.Client
	apiBase      string
	authBase     string
	clientID     string
	clientSecret string
	redirectURI  string
	refreshMu    sync.Mutex
	throttleMu   sync.Mutex
	lastScrobble map[string]scrobbleStamp
}

type scrobbleStamp struct {
	At       time.Time
	Progress float64
	Action   string
}

type Status struct {
	Configured      bool   `json:"configured"`
	Connected       bool   `json:"connected"`
	Username        string `json:"username,omitempty"`
	UserSlug        string `json:"user_slug,omitempty"`
	ConnectedAt     string `json:"connected_at,omitempty"`
	Authorization   bool   `json:"authorization_pending"`
	UserCode        string `json:"user_code,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
}

type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type MediaRef struct {
	MediaID       int64
	MediaType     string
	Title         string
	Year          int
	TMDBID        int64
	Season        int
	Episode       int
	Position      float64
	Duration      float64
	PlaybackState string
}

func New(db *sql.DB, secrets *appsettings.Service, clientID, clientSecret, redirectURI string) *Service {
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		redirectURI = "urn:ietf:wg:oauth:2.0:oob"
	}
	return &Service{
		db: db, secrets: secrets,
		client:       &http.Client{Timeout: 12 * time.Second},
		apiBase:      defaultAPIBase,
		authBase:     defaultAuthBase,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		redirectURI:  redirectURI,
		lastScrobble: map[string]scrobbleStamp{},
	}
}

func (s *Service) Configure(clientID, clientSecret, redirectURI string) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.clientID = strings.TrimSpace(clientID)
	s.clientSecret = strings.TrimSpace(clientSecret)
	if strings.TrimSpace(redirectURI) != "" {
		s.redirectURI = strings.TrimSpace(redirectURI)
	}
}

func (s *Service) Ready() bool {
	return strings.TrimSpace(s.clientID) != "" && strings.TrimSpace(s.clientSecret) != ""
}

func (s *Service) Status(ctx context.Context, profileID int64) (Status, error) {
	out := Status{Configured: s.Ready()}
	var access, username, slug, connectedAt string
	err := s.db.QueryRowContext(ctx, `SELECT access_token,username,user_slug,connected_at FROM profile_trakt WHERE profile_id=?`, profileID).
		Scan(&access, &username, &slug, &connectedAt)
	if err == nil && strings.TrimSpace(access) != "" {
		out.Connected = true
		out.Username = username
		out.UserSlug = slug
		out.ConnectedAt = connectedAt
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}

	var expiresAt string
	err = s.db.QueryRowContext(ctx, `SELECT user_code,verification_url,expires_at,interval_seconds FROM profile_trakt_device_auth WHERE profile_id=?`, profileID).
		Scan(&out.UserCode, &out.VerificationURL, &expiresAt, &out.IntervalSeconds)
	if err == nil {
		if expiry, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil && time.Now().Before(expiry) {
			out.Authorization = true
			out.ExpiresAt = expiresAt
		} else {
			_, _ = s.db.ExecContext(ctx, `DELETE FROM profile_trakt_device_auth WHERE profile_id=?`, profileID)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	return out, nil
}

func (s *Service) BeginDevice(ctx context.Context, profileID int64) (Status, error) {
	if !s.Ready() {
		return Status{Configured: false}, ErrNotConfigured
	}
	var response DeviceCode
	if err := s.postJSON(ctx, s.authBase+"/oauth/device/code", map[string]any{"client_id": s.clientID}, "", &response, nil); err != nil {
		return Status{Configured: true}, err
	}
	if response.DeviceCode == "" || response.UserCode == "" || response.VerificationURL == "" || response.ExpiresIn <= 0 {
		return Status{Configured: true}, errors.New("Trakt returned an incomplete device authorization response")
	}
	if response.Interval <= 0 {
		response.Interval = 5
	}
	expiresAt := time.Now().UTC().Add(time.Duration(response.ExpiresIn) * time.Second).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO profile_trakt_device_auth(profile_id,device_code,user_code,verification_url,expires_at,interval_seconds,requested_at)
VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id) DO UPDATE SET device_code=excluded.device_code,user_code=excluded.user_code,verification_url=excluded.verification_url,expires_at=excluded.expires_at,interval_seconds=excluded.interval_seconds,requested_at=CURRENT_TIMESTAMP`,
		profileID, response.DeviceCode, response.UserCode, response.VerificationURL, expiresAt, response.Interval)
	if err != nil {
		return Status{Configured: true}, err
	}
	return s.Status(ctx, profileID)
}

func (s *Service) PollDevice(ctx context.Context, profileID int64) (Status, error) {
	if !s.Ready() {
		return Status{Configured: false}, ErrNotConfigured
	}
	var deviceCode, expiresAt string
	var interval int
	if err := s.db.QueryRowContext(ctx, `SELECT device_code,expires_at,interval_seconds FROM profile_trakt_device_auth WHERE profile_id=?`, profileID).
		Scan(&deviceCode, &expiresAt, &interval); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.Status(ctx, profileID)
		}
		return Status{Configured: true}, err
	}
	if expiry, err := time.Parse(time.RFC3339, expiresAt); err != nil || time.Now().After(expiry) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM profile_trakt_device_auth WHERE profile_id=?`, profileID)
		return Status{Configured: true}, ErrExpired
	}

	var token TokenResponse
	statusCode := 0
	err := s.postJSON(ctx, s.authBase+"/oauth/device/token", map[string]any{
		"code": deviceCode, "client_id": s.clientID, "client_secret": s.clientSecret,
	}, "", &token, &statusCode)
	if err != nil {
		// Trakt uses non-2xx device-flow responses while approval is pending. The
		// UI should keep polling at the requested interval instead of treating
		// these as a broken account connection.
		if statusCode == http.StatusBadRequest || statusCode == http.StatusTooManyRequests {
			return s.Status(ctx, profileID)
		}
		if statusCode == http.StatusGone || statusCode == http.StatusNotFound {
			_, _ = s.db.ExecContext(ctx, `DELETE FROM profile_trakt_device_auth WHERE profile_id=?`, profileID)
			return Status{Configured: true}, ErrExpired
		}
		return Status{Configured: true}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return Status{Configured: true}, errors.New("Trakt returned an incomplete OAuth token response")
	}
	if err := s.storeTokens(ctx, profileID, token); err != nil {
		return Status{Configured: true}, err
	}
	username, slug, _ := s.fetchUserIdentity(ctx, token.AccessToken)
	_, _ = s.db.ExecContext(ctx, `UPDATE profile_trakt SET username=?,user_slug=?,updated_at=CURRENT_TIMESTAMP WHERE profile_id=?`, username, slug, profileID)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM profile_trakt_device_auth WHERE profile_id=?`, profileID)
	return s.Status(ctx, profileID)
}

func (s *Service) Disconnect(ctx context.Context, profileID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM profile_trakt WHERE profile_id=?`, profileID)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM profile_trakt_device_auth WHERE profile_id=?`, profileID)
	return err
}

func (s *Service) ScrobbleAsync(profileID int64, ref MediaRef) {
	if profileID <= 0 || ref.MediaID <= 0 || ref.TMDBID <= 0 || ref.Duration <= 0 || !s.Ready() {
		return
	}
	progress := ref.Position / ref.Duration * 100
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	action := scrobbleAction(ref.PlaybackState, progress)
	if !s.shouldScrobble(profileID, ref.MediaID, action, progress) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.scrobble(ctx, profileID, ref, action, progress)
	}()
}

func scrobbleAction(state string, progress float64) string {
	state = strings.ToLower(strings.TrimSpace(state))
	switch state {
	case "paused", "pause":
		return "pause"
	case "ended", "finished", "complete", "completed", "stopped", "stop":
		return "stop"
	default:
		if progress >= 99.5 {
			return "stop"
		}
		return "start"
	}
}

func (s *Service) shouldScrobble(profileID, mediaID int64, action string, progress float64) bool {
	key := fmt.Sprintf("%d:%d", profileID, mediaID)
	now := time.Now()
	s.throttleMu.Lock()
	defer s.throttleMu.Unlock()
	last, ok := s.lastScrobble[key]
	if ok && action != "stop" {
		if now.Sub(last.At) < 30*time.Second && last.Action == action && progress-last.Progress < 1.0 {
			return false
		}
	}
	s.lastScrobble[key] = scrobbleStamp{At: now, Progress: progress, Action: action}
	if len(s.lastScrobble) > 4096 {
		cutoff := now.Add(-2 * time.Hour)
		for k, stamp := range s.lastScrobble {
			if stamp.At.Before(cutoff) {
				delete(s.lastScrobble, k)
			}
		}
	}
	return true
}

func (s *Service) scrobble(ctx context.Context, profileID int64, ref MediaRef, action string, progress float64) error {
	token, err := s.accessToken(ctx, profileID)
	if err != nil || token == "" {
		return err
	}
	payload := map[string]any{
		"progress":    progress,
		"app_version": "0.22.0",
		"app_date":    time.Now().UTC().Format("2006-01-02"),
	}
	if ref.MediaType == "movie" {
		payload["movie"] = map[string]any{"title": ref.Title, "year": ref.Year, "ids": map[string]any{"tmdb": ref.TMDBID}}
	} else if ref.Season > 0 && ref.Episode > 0 {
		// StormFlix stores the TMDB show id on episodic rows. Trakt can resolve an
		// episode from show identity plus season/episode numbers, which avoids a
		// second metadata lookup during playback.
		payload["show"] = map[string]any{"title": ref.Title, "year": ref.Year, "ids": map[string]any{"tmdb": ref.TMDBID}}
		payload["episode"] = map[string]any{"season": ref.Season, "number": ref.Episode}
	} else {
		return nil
	}
	return s.postJSON(ctx, s.apiBase+"/scrobble/"+action, payload, token, nil, nil)
}

func (s *Service) accessToken(ctx context.Context, profileID int64) (string, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	var accessEnc, refreshEnc, expiresAt string
	if err := s.db.QueryRowContext(ctx, `SELECT access_token,refresh_token,token_expires_at FROM profile_trakt WHERE profile_id=?`, profileID).
		Scan(&accessEnc, &refreshEnc, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	access, err := s.secrets.OpenSecret(accessEnc)
	if err != nil {
		return "", err
	}
	expiry, parseErr := time.Parse(time.RFC3339, expiresAt)
	if parseErr == nil && time.Now().Add(5*time.Minute).Before(expiry) {
		return access, nil
	}
	refresh, err := s.secrets.OpenSecret(refreshEnc)
	if err != nil {
		return "", err
	}
	if refresh == "" {
		return "", nil
	}
	var token TokenResponse
	if err := s.postJSON(ctx, s.authBase+"/oauth/token", map[string]any{
		"refresh_token": refresh,
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
		"redirect_uri":  s.redirectURI,
		"grant_type":    "refresh_token",
	}, "", &token, nil); err != nil {
		return "", err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		return "", errors.New("Trakt refresh returned incomplete tokens")
	}
	if err := s.storeTokens(ctx, profileID, token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (s *Service) storeTokens(ctx context.Context, profileID int64, token TokenResponse) error {
	access, err := s.secrets.SealSecret(token.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := s.secrets.SealSecret(token.RefreshToken)
	if err != nil {
		return err
	}
	created := time.Now().UTC()
	if token.CreatedAt > 0 {
		created = time.Unix(token.CreatedAt, 0).UTC()
	}
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int64((7 * 24 * time.Hour) / time.Second)
	}
	expiresAt := created.Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
	connectedAt := created.Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO profile_trakt(profile_id,access_token,refresh_token,token_expires_at,connected_at,updated_at)
VALUES(?,?,?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id) DO UPDATE SET access_token=excluded.access_token,refresh_token=excluded.refresh_token,token_expires_at=excluded.token_expires_at,connected_at=CASE WHEN profile_trakt.connected_at='' THEN excluded.connected_at ELSE profile_trakt.connected_at END,updated_at=CURRENT_TIMESTAMP`,
		profileID, access, refresh, expiresAt, connectedAt)
	return err
}

func (s *Service) fetchUserIdentity(ctx context.Context, token string) (string, string, error) {
	var response struct {
		User struct {
			Username string `json:"username"`
			IDs      struct {
				Slug string `json:"slug"`
			} `json:"ids"`
		} `json:"user"`
	}
	if err := s.getJSON(ctx, s.apiBase+"/users/settings", token, &response); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(response.User.Username), strings.TrimSpace(response.User.IDs.Slug), nil
}

func (s *Service) postJSON(ctx context.Context, rawURL string, body any, token string, dest any, statusCode *int) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	s.setHeaders(req, token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if statusCode != nil {
		*statusCode = resp.StatusCode
	}
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Trakt HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if dest != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, dest); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) getJSON(ctx context.Context, rawURL, token string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req, token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("Trakt HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dest)
}

func (s *Service) setHeaders(req *http.Request, token string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.22")
	if s.clientID != "" {
		req.Header.Set("trakt-api-key", s.clientID)
		req.Header.Set("trakt-api-version", "2")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
