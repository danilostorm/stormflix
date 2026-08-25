package config

import "strings"

// NormalizeCredentials accepts common copy/paste formats from provider dashboards.
// In particular, TMDB's read access token is sometimes copied as "Bearer <token>".
// StormFlix stores/sends only the token itself and adds the Authorization scheme.
func NormalizeCredentials(c Config) Config {
	c.TMDBToken = normalizeBearerToken(c.TMDBToken)
	c.TMDBAPIKey = cleanCredential(c.TMDBAPIKey)
	c.FanartAPIKey = cleanCredential(c.FanartAPIKey)
	c.FanartClientKey = cleanCredential(c.FanartClientKey)
	c.SubDLAPIKey = cleanCredential(c.SubDLAPIKey)
	c.OpenSubtitlesAPIKey = cleanCredential(c.OpenSubtitlesAPIKey)
	return c
}

func normalizeBearerToken(value string) string {
	value = cleanCredential(value)
	if len(value) >= 7 && strings.EqualFold(value[:7], "bearer ") {
		value = strings.TrimSpace(value[7:])
	}
	return cleanCredential(value)
}

func cleanCredential(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	return strings.TrimSpace(value)
}
