package media

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	groupedHomeCacheFreshTTL = 2 * time.Minute
	groupedHomeCacheStaleTTL = 10 * time.Minute
)

type groupedHomeCacheEntry struct {
	feed       HomeFeed
	revision   int64
	freshUntil time.Time
	staleUntil time.Time
}

type HomeCacheStatus struct {
	State      string
	Revision   int64
	BuildTime  time.Duration
	Refreshing bool
}

var groupedHomeCache = struct {
	sync.RWMutex
	items      map[string]groupedHomeCacheEntry
	refreshing map[string]bool
}{items: map[string]groupedHomeCacheEntry{}, refreshing: map[string]bool{}}

// HomeGroupedCached keeps repeated Home opens fast even with a very large
// catalog. A fresh snapshot is returned immediately for two minutes. After
// that, the previous valid snapshot remains usable for ten minutes while one
// background refresh rebuilds it. This prevents an ordinary Home navigation
// from synchronously paying for full media/series regrouping every few seconds.
func (s *Service) HomeGroupedCached(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	feed, _, err := s.HomeGroupedCachedWithStatus(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	return feed, err
}

// HomeGroupedCachedWithStatus exposes cache telemetry without changing the
// public Home payload contract used by older clients.
func (s *Service) HomeGroupedCachedWithStatus(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, HomeCacheStatus, error) {
	key := groupedHomeKey(s, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	now := time.Now()
	groupedHomeCache.RLock()
	entry, ok := groupedHomeCache.items[key]
	refreshing := groupedHomeCache.refreshing[key]
	groupedHomeCache.RUnlock()
	projection, projectionErr := s.ensureCatalogProjection(ctx)
	if projectionErr != nil {
		if ok {
			return cloneHomeFeed(entry.feed), HomeCacheStatus{State: "fallback", Revision: entry.revision}, nil
		}
		return HomeFeed{}, HomeCacheStatus{State: "error"}, projectionErr
	}
	revision := projection.BuiltRevision
	if ok && entry.revision == revision && now.Before(entry.freshUntil) {
		return cloneHomeFeed(entry.feed), HomeCacheStatus{State: "hit", Revision: revision, Refreshing: projection.Refreshing || refreshing}, nil
	}
	if ok && now.Before(entry.staleUntil) {
		s.startGroupedHomeRefresh(key, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
		return cloneHomeFeed(entry.feed), HomeCacheStatus{State: "stale", Revision: entry.revision, Refreshing: true}, nil
	}

	started := time.Now()
	feed, err := s.HomeGrouped(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	if err != nil {
		// If a refresh failed during a transient DB/mount condition but we still
		// have an older snapshot, prefer a slightly stale Home over a blank UI.
		if ok {
			return cloneHomeFeed(entry.feed), HomeCacheStatus{State: "fallback", Revision: entry.revision, BuildTime: time.Since(started)}, nil
		}
		return HomeFeed{}, HomeCacheStatus{State: "error", BuildTime: time.Since(started)}, err
	}
	if latest, statusErr := s.CatalogProjectionStatus(ctx); statusErr == nil {
		revision = latest.BuiltRevision
	}
	feed.CacheRevision = revision
	storeGroupedHome(key, feed, revision, time.Now())
	return feed, HomeCacheStatus{State: "miss", Revision: revision, BuildTime: time.Since(started)}, nil
}

func (s *Service) startGroupedHomeRefresh(key string, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) {
	groupedHomeCache.Lock()
	if groupedHomeCache.refreshing[key] {
		groupedHomeCache.Unlock()
		return
	}
	groupedHomeCache.refreshing[key] = true
	groupedHomeCache.Unlock()
	ids := append([]int64(nil), allowedLibraryIDs...)
	if allowedLibraryIDs == nil {
		ids = nil
	}
	go func() {
		defer func() {
			groupedHomeCache.Lock()
			delete(groupedHomeCache.refreshing, key)
			groupedHomeCache.Unlock()
		}()
		ctx, cancel := context.WithTimeout(s.lifecycle, 90*time.Second)
		defer cancel()
		feed, err := s.HomeGrouped(ctx, ids, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
		if err != nil {
			return
		}
		revision := int64(0)
		if projection, statusErr := s.CatalogProjectionStatus(ctx); statusErr == nil {
			revision = projection.BuiltRevision
		}
		feed.CacheRevision = revision
		storeGroupedHome(key, feed, revision, time.Now())
	}()
}

func storeGroupedHome(key string, feed HomeFeed, revision int64, now time.Time) {
	groupedHomeCache.Lock()
	defer groupedHomeCache.Unlock()
	if len(groupedHomeCache.items) > 64 {
		for k, v := range groupedHomeCache.items {
			if now.After(v.staleUntil) {
				delete(groupedHomeCache.items, k)
			}
		}
		if len(groupedHomeCache.items) > 64 {
			groupedHomeCache.items = map[string]groupedHomeCacheEntry{}
		}
	}
	groupedHomeCache.items[key] = groupedHomeCacheEntry{
		feed: cloneHomeFeed(feed), revision: revision, freshUntil: now.Add(groupedHomeCacheFreshTTL), staleUntil: now.Add(groupedHomeCacheStaleTTL),
	}
}

func groupedHomeKey(s *Service, ids []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) string {
	scope := "restricted:"
	if ids == nil {
		scope = "all:"
	}
	copyIDs := append([]int64(nil), ids...)
	sort.Slice(copyIDs, func(i, j int) bool { return copyIDs[i] < copyIDs[j] })
	parts := make([]string, len(copyIDs))
	for i, id := range copyIDs {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return fmt.Sprintf("%p|%s%s|%s|%s|%t|%d|%t", s, scope, strings.Join(parts, ","), heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
}

func cloneHomeFeed(in HomeFeed) HomeFeed {
	out := in
	if in.Hero != nil {
		hero := *in.Hero
		hero.Genres = append([]string(nil), in.Hero.Genres...)
		out.Hero = &hero
	}
	out.Rows = make([]HomeRow, len(in.Rows))
	for i := range in.Rows {
		out.Rows[i] = in.Rows[i]
		out.Rows[i].Items = append([]Item(nil), in.Rows[i].Items...)
		for j := range out.Rows[i].Items {
			out.Rows[i].Items[j].Genres = append([]string(nil), out.Rows[i].Items[j].Genres...)
		}
	}
	return out
}
