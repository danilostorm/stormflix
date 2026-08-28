package httpapi

import (
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
	writeJSON(w, http.StatusOK, s.compatCache.Status())
}

func (s *server) cleanupCompatCache(w http.ResponseWriter, r *http.Request) {
	result, err := s.compatCache.Cleanup(r.Context(), true, "manual-admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "playback", "Compatibility cache cleaned", &uid,
		formatCompatCacheCleanup(result))
	writeJSON(w, http.StatusOK, result)
}

func formatCompatCacheCleanup(result webcompat.CacheCleanupResult) string {
	return "files_removed=" + itoa(result.FilesRemoved) +
		" bytes_removed=" + int64toa(result.BytesRemoved) +
		" usage_before=" + int64toa(result.BytesBefore) +
		" usage_after=" + int64toa(result.BytesAfter) +
		" active_skipped=" + itoa(result.ActiveSkipped)
}
