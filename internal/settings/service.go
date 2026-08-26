package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danilostorm/stormflix/internal/config"
)

type Service struct {
	db  *sql.DB
	key []byte
}

type Public struct {
	ServerName                  string          `json:"server_name"`
	MetadataLanguage            string          `json:"metadata_language"`
	SubtitleLanguages           string          `json:"subtitle_languages"`
	AssetDir                    string          `json:"asset_dir"`
	AssetPublicBaseURL          string          `json:"asset_public_base_url"`
	ThemePreviewEnabled         bool            `json:"theme_preview_enabled"`
	ThemePreviewCountry         string          `json:"theme_preview_country"`
	ThemePreviewVolume          int             `json:"theme_preview_volume"`
	ThemePreviewAutoplay        bool            `json:"theme_preview_autoplay"`
	HomeHeroMode                string          `json:"home_hero_mode"`
	Secrets                     map[string]bool `json:"secrets"`
	OpenSubtitlesUsername       string          `json:"opensubtitles_username"`
	OpenSubtitlesUserAgent      string          `json:"opensubtitles_user_agent"`
}

type Update struct {
	ServerName                 *string `json:"server_name"`
	MetadataLanguage           *string `json:"metadata_language"`
	SubtitleLanguages          *string `json:"subtitle_languages"`
	AssetDir                   *string `json:"asset_dir"`
	AssetPublicBaseURL         *string `json:"asset_public_base_url"`
	ThemePreviewEnabled        *bool   `json:"theme_preview_enabled"`
	ThemePreviewCountry        *string `json:"theme_preview_country"`
	ThemePreviewVolume         *int    `json:"theme_preview_volume"`
	ThemePreviewAutoplay       *bool   `json:"theme_preview_autoplay"`
	HomeHeroMode               *string `json:"home_hero_mode"`
	TMDBToken                  *string `json:"tmdb_token"`
	TMDBAPIKey                 *string `json:"tmdb_api_key"`
	FanartAPIKey               *string `json:"fanart_api_key"`
	FanartClientKey            *string `json:"fanart_client_key"`
	LastFMAPIKey               *string `json:"lastfm_api_key"`
	SubDLAPIKey                *string `json:"subdl_api_key"`
	OpenSubtitlesAPIKey        *string `json:"opensubtitles_api_key"`
	OpenSubtitlesUsername      *string `json:"opensubtitles_username"`
	OpenSubtitlesPassword      *string `json:"opensubtitles_password"`
	OpenSubtitlesUserAgent     *string `json:"opensubtitles_user_agent"`
}

var secretKeys = map[string]bool{
	"tmdb_token": true, "tmdb_api_key": true, "fanart_api_key": true, "fanart_client_key": true,
	"lastfm_api_key": true, "subdl_api_key": true, "opensubtitles_api_key": true, "opensubtitles_password": true,
}

func New(db *sql.DB, dataDir string) (*Service, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dataDir, "settings.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid settings encryption key")
	}
	return &Service{db: db, key: key}, nil
}

func (s *Service) Apply(ctx context.Context, base config.Config) (config.Config, error) {
	values, err := s.all(ctx)
	if err != nil {
		return base, err
	}
	set := func(key string, dest *string) error {
		value, ok := values[key]
		if !ok {
			return nil
		}
		if secretKeys[key] && value != "" {
			value, err = s.decrypt(value)
			if err != nil {
				return fmt.Errorf("decrypt setting %s: %w", key, err)
			}
		}
		*dest = value
		return nil
	}
	for _, pair := range []struct {
		key  string
		dest *string
	}{
		{"server_name", &base.ServerName},
		{"metadata_language", &base.MetadataLanguage},
		{"subtitle_languages", &base.SubtitleLanguages},
		{"asset_dir", &base.AssetDir},
		{"asset_public_base_url", &base.AssetPublicBaseURL},
		{"theme_preview_country", &base.ThemePreviewCountry},
		{"home_hero_mode", &base.HomeHeroMode},
		{"tmdb_token", &base.TMDBToken},
		{"tmdb_api_key", &base.TMDBAPIKey},
		{"fanart_api_key", &base.FanartAPIKey},
		{"fanart_client_key", &base.FanartClientKey},
		{"lastfm_api_key", &base.LastFMAPIKey},
		{"subdl_api_key", &base.SubDLAPIKey},
		{"opensubtitles_api_key", &base.OpenSubtitlesAPIKey},
		{"opensubtitles_username", &base.OpenSubtitlesUsername},
		{"opensubtitles_password", &base.OpenSubtitlesPassword},
		{"opensubtitles_user_agent", &base.OpenSubtitlesUserAgent},
	} {
		if err := set(pair.key, pair.dest); err != nil {
			return base, err
		}
	}
	if value, ok := values["theme_preview_enabled"]; ok {
		base.ThemePreviewEnabled = value == "1" || strings.EqualFold(value, "true")
	}
	if value, ok := values["theme_preview_autoplay"]; ok {
		base.ThemePreviewAutoplay = value == "1" || strings.EqualFold(value, "true")
	}
	if value, ok := values["theme_preview_volume"]; ok {
		if n, err := strconv.Atoi(value); err == nil {
			base.ThemePreviewVolume = clamp(n, 0, 100)
		}
	}
	return base, nil
}

func (s *Service) Public(ctx context.Context, base config.Config) (Public, error) {
	effective, err := s.Apply(ctx, base)
	if err != nil {
		return Public{}, err
	}
	values, err := s.all(ctx)
	if err != nil {
		return Public{}, err
	}
	secrets := map[string]bool{}
	for key := range secretKeys {
		secrets[key] = strings.TrimSpace(values[key]) != ""
	}
	return Public{
		ServerName: effective.ServerName, MetadataLanguage: effective.MetadataLanguage, SubtitleLanguages: effective.SubtitleLanguages,
		AssetDir: effective.AssetDir, AssetPublicBaseURL: effective.AssetPublicBaseURL,
		ThemePreviewEnabled: effective.ThemePreviewEnabled, ThemePreviewCountry: effective.ThemePreviewCountry,
		ThemePreviewVolume: effective.ThemePreviewVolume, ThemePreviewAutoplay: effective.ThemePreviewAutoplay,
		HomeHeroMode: effective.HomeHeroMode, Secrets: secrets,
		OpenSubtitlesUsername: effective.OpenSubtitlesUsername, OpenSubtitlesUserAgent: effective.OpenSubtitlesUserAgent,
	}, nil
}

func (s *Service) Update(ctx context.Context, in Update) error {
	plain := map[string]*string{
		"server_name": in.ServerName, "metadata_language": in.MetadataLanguage, "subtitle_languages": in.SubtitleLanguages,
		"asset_dir": in.AssetDir, "asset_public_base_url": in.AssetPublicBaseURL,
		"theme_preview_country": in.ThemePreviewCountry, "home_hero_mode": in.HomeHeroMode,
		"opensubtitles_username": in.OpenSubtitlesUsername, "opensubtitles_user_agent": in.OpenSubtitlesUserAgent,
	}
	for key, value := range plain {
		if value == nil {
			continue
		}
		if err := s.put(ctx, key, strings.TrimSpace(*value)); err != nil {
			return err
		}
	}
	for key, value := range map[string]*string{
		"tmdb_token": in.TMDBToken, "tmdb_api_key": in.TMDBAPIKey, "fanart_api_key": in.FanartAPIKey,
		"fanart_client_key": in.FanartClientKey, "lastfm_api_key": in.LastFMAPIKey, "subdl_api_key": in.SubDLAPIKey,
		"opensubtitles_api_key": in.OpenSubtitlesAPIKey, "opensubtitles_password": in.OpenSubtitlesPassword,
	} {
		if value == nil {
			continue
		}
		v := strings.TrimSpace(*value)
		if v == "" {
			continue
		}
		if v == "__clear__" {
			if err := s.put(ctx, key, ""); err != nil {
				return err
			}
			continue
		}
		enc, err := s.encrypt(v)
		if err != nil {
			return err
		}
		if err := s.put(ctx, key, enc); err != nil {
			return err
		}
	}
	for key, value := range map[string]*bool{"theme_preview_enabled": in.ThemePreviewEnabled, "theme_preview_autoplay": in.ThemePreviewAutoplay} {
		if value != nil {
			if err := s.put(ctx, key, strconv.FormatBool(*value)); err != nil {
				return err
			}
		}
	}
	if in.ThemePreviewVolume != nil {
		if err := s.put(ctx, "theme_preview_volume", strconv.Itoa(clamp(*in.ThemePreviewVolume, 0, 100))); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) all(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (s *Service) put(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, key, value)
	return err
}

func (s *Service) encrypt(value string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return "enc:v1:" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Service) decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, "enc:v1:") {
		return value, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "enc:v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("encrypted value is truncated")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
