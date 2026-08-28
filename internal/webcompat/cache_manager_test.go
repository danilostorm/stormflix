package webcompat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCacheFile(t *testing.T, dir, name string, size int64, mod time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	return path
}

func testCachePolicy() CachePolicy {
	p := DefaultCachePolicy()
	p.AutoCleanup = false
	p.TTL = 0
	p.MinFreeBytes = 0
	p.MinFreePercent = 0
	p.CleanupInterval = time.Hour
	p.TempMaxAge = time.Hour
	p.OversizeIdleTTL = 10 * time.Minute
	return p
}

func TestCacheBelowLimitIsPreserved(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 200 * 1024
	writeCacheFile(t, dir, "a.mp4", 32*1024, time.Now().Add(-time.Hour))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 0 {
		t.Fatalf("expected no eviction, got %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.mp4")); err != nil {
		t.Fatalf("cache file should remain: %v", err)
	}
}

func TestCacheEvictsLeastRecentlyUsedUntilBelowTarget(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 100 * 1024
	p.EvictionTargetPct = 85
	now := time.Now()
	writeCacheFile(t, dir, "old.mp4", 60*1024, now.Add(-3*time.Hour))
	writeCacheFile(t, dir, "new.mp4", 60*1024, now.Add(-time.Hour))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected one eviction, got %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.mp4")); !os.IsNotExist(err) {
		t.Fatalf("oldest file should be evicted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.mp4")); err != nil {
		t.Fatalf("newest file should remain: %v", err)
	}
}

func TestCacheTTLExpiresOldEntries(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 0
	p.TTL = 30 * time.Minute
	writeCacheFile(t, dir, "expired.mp4", 16*1024, time.Now().Add(-2*time.Hour))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected expired file removal, got %+v", result)
	}
}

func TestActiveCacheFileIsNeverRemoved(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 50 * 1024
	path := writeCacheFile(t, dir, "active.mp4", 80*1024, time.Now().Add(-2*time.Hour))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	release, err := m.Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), true, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveSkipped == 0 {
		t.Fatalf("expected active file to be skipped, got %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active file was removed: %v", err)
	}
	release()
	if _, err := m.Cleanup(context.Background(), true, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("inactive file should be removable after release, stat err=%v", err)
	}
}

func TestAbandonedTempFileIsRemovedButFreshTempIsPreserved(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.TempMaxAge = time.Hour
	old := filepath.Join(dir, "old.mp4.123.tmp")
	fresh := filepath.Join(dir, "fresh.mp4.123.tmp")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.TempRemoved != 1 {
		t.Fatalf("expected one abandoned temp removal, got %+v", result)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh temp should remain: %v", err)
	}
}

func TestActiveMaterializationTempIsNeverRemoved(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.TempMaxAge = time.Minute
	finalPath := writeCacheFile(t, dir, "active.mp4", 16*1024, time.Now())
	tmpPath := filepath.Join(dir, "active.mp4.123.tmp")
	if err := os.WriteFile(tmpPath, []byte("in progress"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(tmpPath, past, past); err != nil {
		t.Fatal(err)
	}
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	release, err := m.Acquire(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.TempRemoved != 0 || result.ActiveSkipped == 0 {
		t.Fatalf("active materialization temp should be preserved, got %+v", result)
	}
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("active temp file was removed: %v", err)
	}
}

func TestOversizeEntryExpiresOnShortIdleTTL(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 20 * 1024
	p.OversizeIdleTTL = 5 * time.Minute
	writeCacheFile(t, dir, "oversize.mp4", 40*1024, time.Now().Add(-20*time.Minute))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected oversize cache to expire, got %+v", result)
	}
}

func TestManualCleanupOnlyTouchesCompatibilityCache(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "compat-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "stormflix.db")
	if err := os.WriteFile(outside, []byte("database"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCacheFile(t, dir, "cache.mp4", 16*1024, time.Now())
	m, err := NewCacheManager(dir, testCachePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cleanup(context.Background(), true, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("cleanup touched data outside compat-cache: %v", err)
	}
}

func TestCacheRejectsPathOutsideDirectory(t *testing.T) {
	dir := t.TempDir()
	m, err := NewCacheManager(dir, testCachePolicy())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(dir), "outside.mp4")
	if _, err := m.Acquire(outside); err == nil {
		t.Fatal("expected outside path to be rejected")
	}
}

func TestUnlimitedCacheDoesNotSizeEvict(t *testing.T) {
	dir := t.TempDir()
	p := testCachePolicy()
	p.MaxBytes = 0
	writeCacheFile(t, dir, "large.mp4", 128*1024, time.Now().Add(-time.Hour))
	m, err := NewCacheManager(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Cleanup(context.Background(), false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 0 {
		t.Fatalf("unlimited cache should not size-evict, got %+v", result)
	}
}
