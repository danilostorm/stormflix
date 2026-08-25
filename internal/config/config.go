package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Address                    string
	DataDir                    string
	MediaRoot                  string
	AssetDir                   string
	AssetPublicBaseURL         string
	ServerName                 string
	MetadataLanguage           string
	TMDBToken                  string
	TMDBAPIKey                 string
	FanartAPIKey               string
	FanartClientKey            string
	SubDLAPIKey                string
	OpenSubtitlesAPIKey        string
	OpenSubtitlesUsername      string
	OpenSubtitlesPassword      string
	OpenSubtitlesUserAgent     string
	SubtitleLanguages          string
	ThemePreviewEnabled        bool
	ThemePreviewCountry        string
	ThemePreviewVolume         int
	ThemePreviewAutoplay       bool
	HomeHeroMode               string
	BootstrapLibraryName       string
	BootstrapLibraryPath       string
}

func Load() Config {
	dataDir := env("STORMFLIX_DATA_DIR", "./data")
	return Config{
		Address:                    env("STORMFLIX_ADDR", ":8090"),
		DataDir:                    dataDir,
		MediaRoot:                  env("STORMFLIX_MEDIA_ROOT", "/media"),
		AssetDir:                   env("STORMFLIX_ASSET_DIR", filepath.Join(dataDir, "assets")),
		AssetPublicBaseURL:         os.Getenv("STORMFLIX_ASSET_PUBLIC_BASE_URL"),
		ServerName:                 env("STORMFLIX_SERVER_NAME", "StormFlix"),
		MetadataLanguage:           env("STORMFLIX_METADATA_LANGUAGE", "pt-BR"),
		TMDBToken:                  os.Getenv("STORMFLIX_TMDB_TOKEN"),
		TMDBAPIKey:                 os.Getenv("STORMFLIX_TMDB_API_KEY"),
		FanartAPIKey:               os.Getenv("STORMFLIX_FANART_API_KEY"),
		FanartClientKey:            os.Getenv("STORMFLIX_FANART_CLIENT_KEY"),
		SubDLAPIKey:                os.Getenv("STORMFLIX_SUBDL_API_KEY"),
		OpenSubtitlesAPIKey:        os.Getenv("STORMFLIX_OPENSUBTITLES_API_KEY"),
		OpenSubtitlesUsername:      os.Getenv("STORMFLIX_OPENSUBTITLES_USERNAME"),
		OpenSubtitlesPassword:      os.Getenv("STORMFLIX_OPENSUBTITLES_PASSWORD"),
		OpenSubtitlesUserAgent:     env("STORMFLIX_OPENSUBTITLES_USER_AGENT", "StormFlix/0.4"),
		SubtitleLanguages:          env("STORMFLIX_SUBTITLE_LANGUAGES", "pt-BR,pt,en"),
		ThemePreviewEnabled:        envBool("STORMFLIX_THEME_PREVIEW_ENABLED", true),
		ThemePreviewCountry:        strings.ToUpper(env("STORMFLIX_THEME_PREVIEW_COUNTRY", "BR")),
		ThemePreviewVolume:         envInt("STORMFLIX_THEME_PREVIEW_VOLUME", 24),
		ThemePreviewAutoplay:       envBool("STORMFLIX_THEME_PREVIEW_AUTOPLAY", true),
		HomeHeroMode:               env("STORMFLIX_HOME_HERO_MODE", "featured"),
		BootstrapLibraryName:       env("STORMFLIX_BOOTSTRAP_LIBRARY_NAME", "Media"),
		BootstrapLibraryPath:       os.Getenv("STORMFLIX_BOOTSTRAP_LIBRARY_PATH"),
	}
}

func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "stormflix.db")
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
