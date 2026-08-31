package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	TVDBAPIKey                 string
	TVDBPIN                    string
	FanartAPIKey               string
	FanartClientKey            string
	LastFMAPIKey               string
	TraktClientID              string
	TraktClientSecret          string
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
	CompatCacheMaxBytes        int64
	CompatCacheTTL             time.Duration
	CompatCacheAutoCleanup     bool
	CompatCacheCleanupInterval time.Duration
	CompatCacheMinFreeBytes    int64
	CompatCacheMinFreePercent  int
	CompatCacheOversizeTTL     time.Duration
	HLSCacheMaxBytes           int64
	HLSCacheIdleTTL            time.Duration
	HLSSegmentDuration         time.Duration
	HLSBatchSegments           int
	BootstrapLibraryName       string
	BootstrapLibraryPath       string
	ManagedMovieLibraryName    string
	ManagedMoviePaths          []string
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
		TVDBAPIKey:                 os.Getenv("STORMFLIX_TVDB_API_KEY"),
		TVDBPIN:                    os.Getenv("STORMFLIX_TVDB_PIN"),
		FanartAPIKey:               os.Getenv("STORMFLIX_FANART_API_KEY"),
		FanartClientKey:            os.Getenv("STORMFLIX_FANART_CLIENT_KEY"),
		LastFMAPIKey:               os.Getenv("STORMFLIX_LASTFM_API_KEY"),
		TraktClientID:              os.Getenv("STORMFLIX_TRAKT_CLIENT_ID"),
		TraktClientSecret:          os.Getenv("STORMFLIX_TRAKT_CLIENT_SECRET"),
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
		CompatCacheMaxBytes:        envInt64("STORMFLIX_COMPAT_CACHE_MAX_BYTES", 20<<30),
		CompatCacheTTL:             envDuration("STORMFLIX_COMPAT_CACHE_TTL", 48*time.Hour),
		CompatCacheAutoCleanup:     envBool("STORMFLIX_COMPAT_CACHE_AUTO_CLEANUP", true),
		CompatCacheCleanupInterval: envDuration("STORMFLIX_COMPAT_CACHE_CLEANUP_INTERVAL", 15*time.Minute),
		CompatCacheMinFreeBytes:    envInt64("STORMFLIX_MIN_FREE_DISK_BYTES", 10<<30),
		CompatCacheMinFreePercent:  envInt("STORMFLIX_MIN_FREE_DISK_PERCENT", 5),
		CompatCacheOversizeTTL:     envDuration("STORMFLIX_COMPAT_CACHE_OVERSIZE_TTL", 15*time.Minute),
		HLSCacheMaxBytes:           envInt64("STORMFLIX_HLS_CACHE_MAX_BYTES", 5<<30),
		HLSCacheIdleTTL:            envDuration("STORMFLIX_HLS_CACHE_IDLE_TTL", 30*time.Minute),
		// Short fragments let Web playback deliver the first frame quickly. The
		// batch still keeps a healthy buffer ahead without materializing a movie.
		HLSSegmentDuration:      envDuration("STORMFLIX_HLS_SEGMENT_DURATION", 2*time.Second),
		HLSBatchSegments:        envInt("STORMFLIX_HLS_BATCH_SEGMENTS", 8),
		BootstrapLibraryName:    env("STORMFLIX_BOOTSTRAP_LIBRARY_NAME", "Media"),
		BootstrapLibraryPath:    os.Getenv("STORMFLIX_BOOTSTRAP_LIBRARY_PATH"),
		ManagedMovieLibraryName: env("STORMFLIX_MANAGED_MOVIE_LIBRARY_NAME", "Filmes"),
		ManagedMoviePaths:       envList("STORMFLIX_MANAGED_MOVIE_PATHS"),
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

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
