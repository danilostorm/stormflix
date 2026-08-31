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
	freshUntil time.Time
	staleUntil time.Time
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
	key := groupedHomeKey(s, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	now := time.Now()
	groupedHomeCache.RLock()
	entry, ok := groupedHomeCache.items[key]
	groupedHomeCache.RUnlock()
	if ok && now.Before(entry.freshUntil) {
		return cloneHomeFeed(entry.feed), nil
	}
	if ok && now.Before(entry.staleUntil) {
		s.startGroupedHomeRefresh(key, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
		return cloneHomeFeed(entry.feed), nil
	}

	feed, err := s.HomeGrouped(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	if err != nil {
		// If a refresh failed during a transient DB/mount condition but we still
		// have an older snapshot, prefer a slightly stale Home over a blank UI.
		if ok {
			return cloneHomeFeed(entry.feed), nil
		}
		return HomeFeed{}, err
	}
	storeGroupedHome(key, feed, now)
	return feed, nil
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
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		feed, err := s.HomeGrouped(ctx, ids, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
		if err != nil {
			return
		}
		storeGroupedHome(key, feed, time.Now())
	}()
}

func storeGroupedHome(key string, feed HomeFeed, now time.Time) {
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
		feed: cloneHomeFeed(feed), freshUntil: now.Add(groupedHomeCacheFreshTTL), staleUntil: now.Add(groupedHomeCacheStaleTTL),
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
