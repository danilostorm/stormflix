package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/webcompat"
)

func compatCachePolicy(cfg config.Config) webcompat.CachePolicy {
	policy := webcompat.DefaultCachePolicy()
	policy.MaxBytes = cfg.CompatCacheMaxBytes
	policy.TTL = cfg.CompatCacheTTL
	policy.AutoCleanup = cfg.CompatCacheAutoCleanup
	policy.CleanupInterval = cfg.CompatCacheCleanupInterval
	policy.MinFreeBytes = cfg.CompatCacheMinFreeBytes
	policy.MinFreePercent = cfg.CompatCacheMinFreePercent
	policy.OversizeIdleTTL = cfg.CompatCacheOversizeTTL
	return policy
}

func newCompatCache(cfg config.Config) (*webcompat.CacheManager, error) {
	return webcompat.NewCacheManager(filepath.Join(cfg.DataDir, "compat-cache"), compatCachePolicy(cfg))
}

func (s *server) compatCacheStatus(w http.ResponseWriter, r *http.Request) {
	// Embed the legacy fields to keep the existing Admin UI/API contract stable
	// while exposing the new session-scoped Web HLS cache alongside it.
	writeJSON(w, http.StatusOK, struct {
		webcompat.CacheStatus
		HLS webcompat.HLSStatus `json:"hls"`
	}{CacheStatus: s.compatCache.Status(), HLS: s.hlsCache.Status()})
}

func (s *server) cleanupCompatCache(w http.ResponseWriter, r *http.Request) {
	result, err := s.compatCache.Cleanup(r.Context(), true, "manual-admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// HLS normal lifecycle cleanup is immediate on playback close. Manual Admin
	// cleanup only applies stale/pressure cleanup and never terminates a healthy
	// active movie session.
	if err := s.hlsCache.Cleanup(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "playback", "Playback caches cleaned", &uid,
		formatCompatCacheCleanup(result))
	writeJSON(w, http.StatusOK, struct {
		webcompat.CacheCleanupResult
		HLS webcompat.HLSStatus `json:"hls"`
	}{CacheCleanupResult: result, HLS: s.hlsCache.Status()})
}

func formatCompatCacheCleanup(result webcompat.CacheCleanupResult) string {
	return fmt.Sprintf("files_removed=%d bytes_removed=%d usage_before=%d usage_after=%d active_skipped=%d",
		result.FilesRemoved, result.BytesRemoved, result.BytesBefore, result.BytesAfter, result.ActiveSkipped)
}
