package media

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// HomeGrouped keeps file-level episodes out of the main catalog and replaces
// them with top-level series cards. It preserves the cinema feed for movies
// while adding dedicated rails for series and recent episodes.
func (s *Service) HomeGrouped(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	// The flat catalog and series grouping are independent DB reads. On a cold
	// cache they used to run one after the other, which made large video
	// libraries spend most of the first Home request on a black screen. Run the
	// two expensive reads concurrently and join them before composing the feed.
	var feed HomeFeed
	var groups []SeriesDetail
	var homeErr, groupsErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		feed, homeErr = s.Home(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	}()
	go func() {
		defer wg.Done()
		groups, groupsErr = s.seriesGroups(ctx, allowedLibraryIDs)
	}()
	wg.Wait()
	if homeErr != nil {
		return HomeFeed{}, homeErr
	}
	if groupsErr != nil {
		return HomeFeed{}, groupsErr
	}

	seriesCards := make([]Item, 0, len(groups))
	allEpisodes := []Item{}
	episodeToSeries := map[int64]Item{}
	for _, group := range groups {
		card := seriesToItem(group.SeriesSummary)
		seriesCards = append(seriesCards, card)
		for _, season := range group.Seasons {
			for _, episode := range season.Episodes {
				allEpisodes = append(allEpisodes, episode)
				episodeToSeries[episode.ID] = card
			}
		}
	}

	for i := range feed.Rows {
		row := &feed.Rows[i]
		kept := make([]Item, 0, len(row.Items))
		seenSeries := map[string]bool{}
		seenMedia := map[int64]bool{}
		for _, item := range row.Items {
			if show, ok := episodeToSeries[item.ID]; ok {
				if !seenSeries[show.SeriesID] {
					seenSeries[show.SeriesID] = true
					kept = append(kept, show)
				}
				continue
			}
			if isEpisodeItem(item) {
				continue
			}
			if !seenMedia[item.ID] {
				seenMedia[item.ID] = true
				kept = append(kept, item)
			}
		}
		row.Items = uniqueCatalogItems(kept)
	}

	// Rebuild the first "recent" rail with top-level entities only.
	topLevel := []Item{}
	for _, row := range feed.Rows {
		for _, item := range row.Items {
			if _, rawEpisode := episodeToSeries[item.ID]; !rawEpisode {
				topLevel = append(topLevel, item)
			}
		}
	}
	topLevel = uniqueCatalogItems(topLevel)
	sort.SliceStable(topLevel, func(i, j int) bool { return topLevel[i].ModifiedUnix > topLevel[j].ModifiedUnix })
	if len(topLevel) > 24 {
		topLevel = topLevel[:24]
	}
	for i := range feed.Rows {
		if feed.Rows[i].ID == "recent" {
			feed.Rows[i].Items = topLevel
			break
		}
	}

	rows := make([]HomeRow, 0, len(feed.Rows)+2)
	if len(seriesCards) > 0 {
		sort.SliceStable(seriesCards, func(i, j int) bool { return seriesCards[i].ModifiedUnix > seriesCards[j].ModifiedUnix })
		rows = append(rows, HomeRow{ID: "series-recent", Title: "Séries adicionadas recentemente", Items: capItems(seriesCards, 24)})
	}
	if len(allEpisodes) > 0 {
		sort.SliceStable(allEpisodes, func(i, j int) bool { return allEpisodes[i].ModifiedUnix > allEpisodes[j].ModifiedUnix })
		rows = append(rows, HomeRow{ID: "episode-recent", Title: "Novos episódios", Items: capItems(allEpisodes, 24)})
	}
	rows = append(rows, feed.Rows...)
	feed.Rows = dedupeHomeRows(rows)

	// Never feature a raw episode in the main hero, including items that have
	// not received provider metadata yet.
	if feed.Hero != nil {
		if show, ok := episodeToSeries[feed.Hero.ID]; ok {
			candidate := show
			feed.Hero = &candidate
		} else if isEpisodeItem(*feed.Hero) {
			if len(topLevel) > 0 {
				candidate := topLevel[0]
				feed.Hero = &candidate
			} else {
				feed.Hero = nil
			}
		}
	}
	return feed, nil
}

func seriesToItem(show SeriesSummary) Item {
	return Item{
		ID: show.RepresentativeMediaID, LibraryID: show.LibraryID, LibraryName: show.LibraryName, Title: show.Title,
		ModifiedUnix: show.ModifiedUnix, Available: true, MediaType: show.MediaType, Year: show.Year, Overview: show.Overview,
		Genres: append([]string(nil), show.Genres...), Rating: show.Rating, PosterURL: show.PosterURL, BackdropURL: show.BackdropURL, LogoURL: show.LogoURL,
		EntityType: "series", SeriesID: show.ID, SeasonCount: show.SeasonCount, EpisodeCount: show.EpisodeCount,
	}
}

func isEpisodeItem(item Item) bool {
	return item.EpisodeNumber > 0 || (item.SeasonNumber > 0 && (item.MediaType == "series" || item.MediaType == "anime"))
}

func uniqueCatalogItems(items []Item) []Item {
	out := make([]Item, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		key := "media:" + strings.TrimSpace(item.LibraryName) + ":" + strings.TrimSpace(item.Title)
		if item.EntityType == "series" && item.SeriesID != "" {
			key = "series:" + item.SeriesID
		}
		if item.EntityType != "series" {
			key = "media-id:" + strconv.FormatInt(item.ID, 10)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func dedupeHomeRows(rows []HomeRow) []HomeRow {
	out := make([]HomeRow, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		row.Items = uniqueCatalogItems(row.Items)
		if len(row.Items) == 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(row.ID))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(row.Title))
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, row)
	}
	return out
}
