package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Address              string
	DataDir              string
	MediaRoot            string
	BootstrapLibraryName string
	BootstrapLibraryPath string
}

func Load() Config {
	return Config{
		Address:              env("STORMFLIX_ADDR", ":8090"),
		DataDir:              env("STORMFLIX_DATA_DIR", "./data"),
		MediaRoot:            env("STORMFLIX_MEDIA_ROOT", "/media"),
		BootstrapLibraryName: env("STORMFLIX_BOOTSTRAP_LIBRARY_NAME", "Media"),
		BootstrapLibraryPath: os.Getenv("STORMFLIX_BOOTSTRAP_LIBRARY_PATH"),
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
