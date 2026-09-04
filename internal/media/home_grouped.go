package media

import (
	"context"
	"strconv"
	"strings"
)

// HomeGrouped keeps file-level episodes out of the main catalog and replaces
// them with top-level series cards. It preserves the cinema feed for movies
// while adding dedicated rails for series and recent episodes.
func (s *Service) HomeGrouped(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	// Home reads a rebuildable logical projection instead of walking every
	// physical episode, artwork row and duplicate source on every request.
	// Projection swaps are atomic, so readers keep receiving the previous good
	// snapshot while a library scan refreshes the next one in background.
	feed, err := s.homeQueryV2(ctx, allowedLibraryIDs, heroMode, serverName, themeEnabled, themeVolume, themeAutoplay)
	if err != nil {
		return HomeFeed{}, err
	}
	allEpisodes, err := s.recentEpisodesForHome(ctx, allowedLibraryIDs, 24)
	if err != nil {
		return HomeFeed{}, err
	}

	rows := make([]HomeRow, 0, len(feed.Rows)+1)
	for _, row := range feed.Rows {
		rows = append(rows, row)
		if row.ID == "series-recent" && len(allEpisodes) > 0 {
			rows = append(rows, HomeRow{ID: "episode-recent", Title: "Novos episódios", Items: capItems(allEpisodes, 24)})
			allEpisodes = nil
		}
	}
	if len(allEpisodes) > 0 {
		rows = append([]HomeRow{{ID: "episode-recent", Title: "Novos episódios", Items: capItems(allEpisodes, 24)}}, rows...)
	}
	feed.Rows = dedupeHomeRows(rows)
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
