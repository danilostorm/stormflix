package media

import "context"

type EpisodeNeighbors struct {
	SeriesID    string `json:"series_id,omitempty"`
	SeriesTitle string `json:"series_title,omitempty"`
	Previous    *Item  `json:"previous,omitempty"`
	Next        *Item  `json:"next,omitempty"`
}

// EpisodeNeighbors returns the adjacent episodes for a catalog media item.
// It intentionally reuses the same series grouping logic used by /series so
// Android, TV and Fire TV do not need a second series identity algorithm.
func (s *Service) EpisodeNeighbors(ctx context.Context, mediaID int64, allowedLibraryIDs []int64) (EpisodeNeighbors, error) {
	out := EpisodeNeighbors{}
	groups, err := s.seriesGroups(ctx, allowedLibraryIDs)
	if err != nil {
		return out, err
	}
	for _, group := range groups {
		episodes := []Item{}
		for _, season := range group.Seasons {
			episodes = append(episodes, season.Episodes...)
		}
		for i := range episodes {
			if episodes[i].ID != mediaID {
				continue
			}
			out.SeriesID = group.ID
			out.SeriesTitle = group.Title
			if i > 0 {
				previous := episodes[i-1]
				out.Previous = &previous
			}
			if i+1 < len(episodes) {
				next := episodes[i+1]
				out.Next = &next
			}
			return out, nil
		}
	}
	return out, nil
}
