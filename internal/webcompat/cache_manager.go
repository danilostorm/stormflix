package webcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const cacheManifestName = ".stormflix-cache.json"

type CachePolicy struct {
	MaxBytes          int64
	TTL               time.Duration
	AutoCleanup       bool
	CleanupInterval   time.Duration
	TempMaxAge        time.Duration
	OversizeIdleTTL   time.Duration
	MinFreeBytes      int64
	MinFreePercent    int
	EvictionTargetPct int
}

func DefaultCachePolicy() CachePolicy {
	return CachePolicy{
		MaxBytes:          20 << 30,
		TTL:               48 * time.Hour,
		AutoCleanup:       true,
		CleanupInterval:   15 * time.Minute,
		TempMaxAge:        time.Hour,
		OversizeIdleTTL:   15 * time.Minute,
		MinFreeBytes:      10 << 30,
		MinFreePercent:    5,
		EvictionTargetPct: 85,
	}
}

type CacheEntry struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	Oversize   bool      `json:"oversize,omitempty"`
}

type CacheCleanupResult struct {
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Reason        string    `json:"reason"`
	FilesBefore   int       `json:"files_before"`
	FilesRemoved  int       `json:"files_removed"`
	BytesBefore   int64     `json:"bytes_before"`
	BytesRemoved  int64     `json:"bytes_removed"`
	BytesAfter    int64     `json:"bytes_after"`
	TempRemoved   int       `json:"temp_removed"`
	ActiveSkipped int       `json:"active_skipped"`
}

type CacheStatus struct {
	Directory        string             `json:"directory"`
	UsageBytes       int64              `json:"usage_bytes"`
	MaxBytes         int64              `json:"max_bytes"`
	TTLSeconds       int64              `json:"ttl_seconds"`
	AutoCleanup      bool               `json:"auto_cleanup"`
	Files            int                `json:"files"`
	ActiveFiles      int                `json:"active_files"`
	OldestLastUsedAt *time.Time         `json:"oldest_last_used_at,omitempty"`
	LastCleanup      CacheCleanupResult `json:"last_cleanup"`
	FreeBytes        int64              `json:"free_bytes"`
	MinFreeBytes     int64              `json:"min_free_bytes"`
	MinFreePercent   int                `json:"min_free_percent"`
}

type cacheManifest struct {
	Version int                   `json:"version"`
	Entries map[string]CacheEntry `json:"entries"`
}

type CacheManager struct {
	dir string

	mu          sync.Mutex
	policy      CachePolicy
	entries     map[string]CacheEntry
	active      map[string]int
	dirty       bool
	lastPersist time.Time
	lastCleanup CacheCleanupResult
	started     bool
}

func NewCacheManager(dir string, policy CachePolicy) (*CacheManager, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("compatibility cache directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create compatibility cache directory: %w", err)
	}
	m := &CacheManager{
		dir:     filepath.Clean(dir),
		policy:  normalizeCachePolicy(policy),
		entries: map[string]CacheEntry{},
		active:  map[string]int{},
	}
	if err := m.loadManifest(); err != nil {
		log.Printf("stormflix compat cache: manifest ignored: %v", err)
	}
	if err := m.reconcile(); err != nil {
		return nil, err
	}
	return m, nil
}

func normalizeCachePolicy(policy CachePolicy) CachePolicy {
	defaults := DefaultCachePolicy()
	if policy.CleanupInterval <= 0 {
		policy.CleanupInterval = defaults.CleanupInterval
	}
	if policy.TempMaxAge <= 0 {
		policy.TempMaxAge = defaults.TempMaxAge
	}
	if policy.OversizeIdleTTL <= 0 {
		policy.OversizeIdleTTL = defaults.OversizeIdleTTL
	}
	if policy.MinFreePercent < 0 {
		policy.MinFreePercent = 0
	}
	if policy.MinFreePercent > 95 {
		policy.MinFreePercent = 95
	}
	if policy.EvictionTargetPct <= 0 || policy.EvictionTargetPct >= 100 {
		policy.EvictionTargetPct = defaults.EvictionTargetPct
	}
	if policy.MaxBytes < 0 {
		policy.MaxBytes = 0
	}
	if policy.TTL < 0 {
		policy.TTL = 0
	}
	if policy.MinFreeBytes < 0 {
		policy.MinFreeBytes = 0
	}
	return policy
}

func (m *CacheManager) Configure(policy CachePolicy) {
	m.mu.Lock()
	m.policy = normalizeCachePolicy(policy)
	m.mu.Unlock()
	if policy.AutoCleanup {
		go func() {
			if _, err := m.Cleanup(context.Background(), false, "configuration"); err != nil {
				log.Printf("stormflix compat cache cleanup after configuration: %v", err)
			}
		}()
	}
}

func (m *CacheManager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	interval := m.policy.CleanupInterval
	m.mu.Unlock()

	go func() {
		cleanupTicker := time.NewTicker(interval)
		flushTicker := time.NewTicker(30 * time.Second)
		defer cleanupTicker.Stop()
		defer flushTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = m.persist()
				return
			case <-flushTicker.C:
				_ = m.persist()
			case <-cleanupTicker.C:
				m.mu.Lock()
				auto := m.policy.AutoCleanup
				m.mu.Unlock()
				if auto {
					if _, err := m.Cleanup(context.Background(), false, "periodic"); err != nil {
						log.Printf("stormflix compat cache periodic cleanup: %v", err)
					}
				}
			}
		}
	}()

	go func() {
		if _, err := m.Cleanup(context.Background(), false, "startup"); err != nil {
			log.Printf("stormflix compat cache startup cleanup: %v", err)
		}
	}()
}

func (m *CacheManager) Directory() string { return m.dir }

func (m *CacheManager) MaterializeSeekable(ctx context.Context, source string, plan Plan, cacheKey string) (string, error) {
	finalPath := MaterializedPath(m.dir, cacheKey)
	if err := m.begin(finalPath); err != nil {
		return "", err
	}
	defer m.end(finalPath)

	if stat, err := os.Stat(finalPath); err == nil && stat.Size() > 4096 {
		path, err := MaterializeSeekable(ctx, source, plan, m.dir, cacheKey)
		if err == nil {
			m.record(path, stat.Size())
		}
		return path, err
	}

	estimated := int64(0)
	if stat, err := os.Stat(source); err == nil && stat.Mode().IsRegular() {
		estimated = stat.Size()
	}
	if err := m.ensureCapacity(estimated); err != nil {
		return "", err
	}
	path, err := MaterializeSeekable(ctx, source, plan, m.dir, cacheKey)
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	m.record(path, stat.Size())
	go func() {
		if _, err := m.Cleanup(context.Background(), false, "post-materialize"); err != nil {
			log.Printf("stormflix compat cache post-materialize cleanup: %v", err)
		}
	}()
	return path, nil
}

func (m *CacheManager) Acquire(path string) (func(), error) {
	name, err := m.nameForPath(path)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.active[name]++
	m.touchLocked(name, time.Now())
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		if m.active[name] <= 1 {
			delete(m.active, name)
		} else {
			m.active[name]--
		}
		m.touchLocked(name, time.Now())
		m.mu.Unlock()
	}, nil
}

func (m *CacheManager) Touch(path string) {
	name, err := m.nameForPath(path)
	if err != nil {
		return
	}
	m.mu.Lock()
	m.touchLocked(name, time.Now())
	m.mu.Unlock()
}

func (m *CacheManager) Status() CacheStatus {
	_ = m.reconcile()
	m.mu.Lock()
	defer m.mu.Unlock()
	status := CacheStatus{
		Directory:      m.dir,
		MaxBytes:       m.policy.MaxBytes,
		TTLSeconds:     int64(m.policy.TTL / time.Second),
		AutoCleanup:    m.policy.AutoCleanup,
		MinFreeBytes:   m.policy.MinFreeBytes,
		MinFreePercent: m.policy.MinFreePercent,
		LastCleanup:    m.lastCleanup,
	}
	for name, entry := range m.entries {
		status.UsageBytes += entry.SizeBytes
		status.Files++
		if m.active[name] > 0 {
			status.ActiveFiles++
		}
		if status.OldestLastUsedAt == nil || entry.LastUsedAt.Before(*status.OldestLastUsedAt) {
			t := entry.LastUsedAt
			status.OldestLastUsedAt = &t
		}
	}
	if free, _, err := diskSpace(m.dir); err == nil {
		status.FreeBytes = free
	}
	return status
}

func (m *CacheManager) Cleanup(ctx context.Context, manual bool, reason string) (CacheCleanupResult, error) {
	if reason == "" {
		if manual {
			reason = "manual"
		} else {
			reason = "automatic"
		}
	}
	if err := m.reconcile(); err != nil {
		return CacheCleanupResult{}, err
	}
	now := time.Now()
	result := CacheCleanupResult{StartedAt: now, Reason: reason}

	m.mu.Lock()
	defer m.mu.Unlock()
	result.FilesBefore = len(m.entries)
	for _, entry := range m.entries {
		result.BytesBefore += entry.SizeBytes
	}

	if err := m.cleanupTempsLocked(now, &result); err != nil {
		return result, err
	}

	type candidate struct {
		name  string
		entry CacheEntry
	}
	candidates := make([]candidate, 0, len(m.entries))
	for name, entry := range m.entries {
		if m.active[name] > 0 {
			result.ActiveSkipped++
			continue
		}
		remove := manual || entry.SizeBytes <= 4096
		if !remove && m.policy.TTL > 0 && now.Sub(entry.LastUsedAt) >= m.policy.TTL {
			remove = true
		}
		if !remove && entry.Oversize && now.Sub(entry.LastUsedAt) >= m.policy.OversizeIdleTTL {
			remove = true
		}
		if remove {
			if err := m.removeLocked(name, &result); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("stormflix compat cache: remove %s: %v", name, err)
			}
			continue
		}
		candidates = append(candidates, candidate{name: name, entry: entry})
	}

	usage := int64(0)
	for _, entry := range m.entries {
		usage += entry.SizeBytes
	}
	if !manual && m.policy.MaxBytes > 0 && usage > m.policy.MaxBytes {
		target := m.policy.MaxBytes * int64(m.policy.EvictionTargetPct) / 100
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].entry.LastUsedAt.Before(candidates[j].entry.LastUsedAt) })
		for _, c := range candidates {
			if usage <= target {
				break
			}
			entry, ok := m.entries[c.name]
			if !ok || m.active[c.name] > 0 {
				continue
			}
			if err := m.removeLocked(c.name, &result); err == nil || errors.Is(err, os.ErrNotExist) {
				usage -= entry.SizeBytes
			}
		}
	}

	result.BytesAfter = 0
	for _, entry := range m.entries {
		result.BytesAfter += entry.SizeBytes
	}
	result.FinishedAt = time.Now()
	m.lastCleanup = result
	m.dirty = true
	if err := m.persistLocked(); err != nil {
		return result, err
	}
	log.Printf("stormflix compat cache cleanup completed reason=%s files_removed=%d bytes_removed=%d usage_before=%d usage_after=%d active_skipped=%d", result.Reason, result.FilesRemoved, result.BytesRemoved, result.BytesBefore, result.BytesAfter, result.ActiveSkipped)
	return result, nil
}

func (m *CacheManager) ensureCapacity(estimated int64) error {
	if estimated < 0 {
		estimated = 0
	}
	if _, err := m.Cleanup(context.Background(), false, "capacity-check"); err != nil {
		return err
	}

	m.mu.Lock()
	policy := m.policy
	usage := int64(0)
	for _, entry := range m.entries {
		usage += entry.SizeBytes
	}
	m.mu.Unlock()

	if policy.MaxBytes > 0 && estimated > 0 && estimated <= policy.MaxBytes && usage+estimated > policy.MaxBytes {
		need := usage + estimated - (policy.MaxBytes * int64(policy.EvictionTargetPct) / 100)
		if need > 0 {
			if _, err := m.evictBytes(need, "size-reserve"); err != nil {
				return err
			}
		}
	}

	free, total, err := diskSpace(m.dir)
	if err != nil {
		return nil
	}
	reserve := policy.MinFreeBytes
	if policy.MinFreePercent > 0 && total > 0 {
		percentReserve := total * int64(policy.MinFreePercent) / 100
		if percentReserve > reserve {
			reserve = percentReserve
		}
	}
	required := reserve + estimated
	if free >= required {
		return nil
	}
	log.Printf("stormflix compat cache disk pressure detected free=%d required=%d estimate=%d", free, required, estimated)
	if _, err := m.evictBytes(required-free, "disk-pressure"); err != nil {
		return err
	}
	free, _, err = diskSpace(m.dir)
	if err == nil && free < required {
		return fmt.Errorf("not enough free disk space for compatibility playback: need about %d bytes free including reserve, have %d", required, free)
	}
	return nil
}

func (m *CacheManager) evictBytes(bytesNeeded int64, reason string) (CacheCleanupResult, error) {
	result := CacheCleanupResult{StartedAt: time.Now(), Reason: reason}
	m.mu.Lock()
	defer m.mu.Unlock()
	result.FilesBefore = len(m.entries)
	for _, entry := range m.entries {
		result.BytesBefore += entry.SizeBytes
	}
	type candidate struct {
		name  string
		entry CacheEntry
	}
	list := make([]candidate, 0, len(m.entries))
	for name, entry := range m.entries {
		if m.active[name] > 0 {
			result.ActiveSkipped++
			continue
		}
		list = append(list, candidate{name: name, entry: entry})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].entry.LastUsedAt.Before(list[j].entry.LastUsedAt) })
	removed := int64(0)
	for _, c := range list {
		if removed >= bytesNeeded {
			break
		}
		entry, ok := m.entries[c.name]
		if !ok || m.active[c.name] > 0 {
			continue
		}
		if err := m.removeLocked(c.name, &result); err == nil || errors.Is(err, os.ErrNotExist) {
			removed += entry.SizeBytes
		}
	}
	result.BytesAfter = result.BytesBefore - result.BytesRemoved
	if result.BytesAfter < 0 {
		result.BytesAfter = 0
	}
	result.FinishedAt = time.Now()
	m.lastCleanup = result
	m.dirty = true
	if err := m.persistLocked(); err != nil {
		return result, err
	}
	return result, nil
}

func (m *CacheManager) begin(path string) error {
	name, err := m.nameForPath(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.active[name]++
	m.mu.Unlock()
	return nil
}

func (m *CacheManager) end(path string) {
	name, err := m.nameForPath(path)
	if err != nil {
		return
	}
	m.mu.Lock()
	if m.active[name] <= 1 {
		delete(m.active, name)
	} else {
		m.active[name]--
	}
	m.mu.Unlock()
}

func (m *CacheManager) record(path string, size int64) {
	name, err := m.nameForPath(path)
	if err != nil {
		return
	}
	now := time.Now()
	m.mu.Lock()
	entry, ok := m.entries[name]
	if !ok {
		entry = CacheEntry{Name: name, CreatedAt: now}
	}
	entry.SizeBytes = size
	entry.LastUsedAt = now
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.Oversize = m.policy.MaxBytes > 0 && size > m.policy.MaxBytes
	m.entries[name] = entry
	m.dirty = true
	m.mu.Unlock()
}

func (m *CacheManager) touchLocked(name string, now time.Time) {
	entry, ok := m.entries[name]
	if !ok {
		path, err := m.safePath(name)
		if err != nil {
			return
		}
		stat, err := os.Stat(path)
		if err != nil {
			return
		}
		entry = CacheEntry{Name: name, SizeBytes: stat.Size(), CreatedAt: stat.ModTime(), Oversize: m.policy.MaxBytes > 0 && stat.Size() > m.policy.MaxBytes}
	}
	entry.LastUsedAt = now
	m.entries[name] = entry
	m.dirty = true
}

func (m *CacheManager) removeLocked(name string, result *CacheCleanupResult) error {
	if m.active[name] > 0 {
		result.ActiveSkipped++
		return nil
	}
	path, err := m.safePath(name)
	if err != nil {
		return err
	}
	entry := m.entries[name]
	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(m.entries, name)
	result.FilesRemoved++
	result.BytesRemoved += entry.SizeBytes
	m.dirty = true
	return nil
}

func (m *CacheManager) cleanupTempsLocked(now time.Time, result *CacheCleanupResult) error {
	items, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".tmp") {
			continue
		}
		info, err := item.Info()
		if err != nil || now.Sub(info.ModTime()) < m.policy.TempMaxAge {
			continue
		}
		path := filepath.Join(m.dir, item.Name())
		if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			result.TempRemoved++
		}
	}
	return nil
}

func (m *CacheManager) reconcile() error {
	items, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	seen := map[string]bool{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != ".mp4" {
			continue
		}
		path, err := m.safePath(item.Name())
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		seen[item.Name()] = true
		entry, ok := m.entries[item.Name()]
		if !ok {
			last := info.ModTime()
			if last.IsZero() {
				last = now
			}
			entry = CacheEntry{Name: item.Name(), CreatedAt: info.ModTime(), LastUsedAt: last}
			m.dirty = true
		}
		entry.SizeBytes = info.Size()
		entry.Oversize = m.policy.MaxBytes > 0 && info.Size() > m.policy.MaxBytes
		if entry.LastUsedAt.IsZero() {
			entry.LastUsedAt = info.ModTime()
		}
		m.entries[item.Name()] = entry
	}
	for name := range m.entries {
		if !seen[name] && m.active[name] == 0 {
			delete(m.entries, name)
			m.dirty = true
		}
	}
	return nil
}

func (m *CacheManager) loadManifest() error {
	data, err := os.ReadFile(filepath.Join(m.dir, cacheManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var manifest cacheManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Entries != nil {
		m.entries = manifest.Entries
	}
	return nil
}

func (m *CacheManager) persist() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistLocked()
}

func (m *CacheManager) persistLocked() error {
	if !m.dirty && !m.lastPersist.IsZero() {
		return nil
	}
	manifest := cacheManifest{Version: 1, Entries: m.entries}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.dir, cacheManifestName+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(m.dir, cacheManifestName)); err != nil {
		return err
	}
	m.dirty = false
	m.lastPersist = time.Now()
	return nil
}

func (m *CacheManager) nameForPath(path string) (string, error) {
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(m.dir, clean)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", errors.New("compatibility cache path escapes cache directory")
	}
	if filepath.Dir(rel) != "." || filepath.Ext(rel) != ".mp4" {
		return "", errors.New("invalid compatibility cache path")
	}
	return rel, nil
}

func (m *CacheManager) safePath(name string) (string, error) {
	if filepath.Base(name) != name || filepath.Ext(name) != ".mp4" || strings.Contains(name, "..") {
		return "", errors.New("invalid compatibility cache entry")
	}
	path := filepath.Join(m.dir, name)
	if _, err := m.nameForPath(path); err != nil {
		return "", err
	}
	return path, nil
}

func diskSpace(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	free = int64(stat.Bavail) * int64(stat.Bsize)
	total = int64(stat.Blocks) * int64(stat.Bsize)
	return free, total, nil
}
