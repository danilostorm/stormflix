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

const groupedHomeCacheTTL = 20 * time.Second

type groupedHomeCacheEntry struct {
	feed      HomeFeed
	expiresAt time.Time
}

var groupedHomeCache = struct {
	sync.RWMutex
	items map[string]groupedHomeCacheEntry
}{items: map[string]groupedHomeCacheEntry{}}

// HomeGroupedCached caches only the catalog-derived/static part of Home.
// Continue Watching and discovery rails stay outside this cache because they
// are profile/time-sensitive. Twenty seconds is enough to prevent repeated
// full series grouping during navigation/reloads without making scans feel
// stale to an administrator.
func (s *Service) HomeGroupedCached(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	key := groupedHomeKey(s, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	now := time.Now()
	groupedHomeCache.RLock()
	entry, ok := groupedHomeCache.items[key]
	groupedHomeCache.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return cloneHomeFeed(entry.feed), nil
	}

	feed, err := s.HomeGrouped(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	if err != nil {
		return HomeFeed{}, err
	}
	groupedHomeCache.Lock()
	if len(groupedHomeCache.items) > 64 {
		for k, v := range groupedHomeCache.items {
			if now.After(v.expiresAt) {
				delete(groupedHomeCache.items, k)
			}
		}
		if len(groupedHomeCache.items) > 64 {
			groupedHomeCache.items = map[string]groupedHomeCacheEntry{}
		}
	}
	groupedHomeCache.items[key] = groupedHomeCacheEntry{feed: cloneHomeFeed(feed), expiresAt: now.Add(groupedHomeCacheTTL)}
	groupedHomeCache.Unlock()
	return feed, nil
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
			out.Rows[i].Items[j].Genres = append([]string(nil), in.Rows[i].Items[j].Genres...)
		}
	}
	return out
}
