package media

import (
	"fmt"
	"strings"
	"unicode"
)

// DedupeItems collapses physical copies of the same title into a single
// catalog entity. The underlying media rows are intentionally preserved: they
// are playback sources/servers and are exposed through Versions().
func DedupeItems(items []Item) []Item {
	if len(items) < 2 {
		return items
	}
	out := make([]Item, 0, len(items))
	index := map[string]int{}
	for _, item := range items {
		key := logicalItemKey(item)
		if key == "" {
			out = append(out, item)
			continue
		}
		if pos, ok := index[key]; ok {
			if itemPresentationScore(item) > itemPresentationScore(out[pos]) {
				out[pos] = item
			}
			continue
		}
		index[key] = len(out)
		out = append(out, item)
	}
	return out
}

func logicalItemKey(item Item) string {
	mediaType := strings.ToLower(strings.TrimSpace(item.MediaType))
	if item.TMDBID > 0 {
		if item.EpisodeNumber > 0 {
			return fmt.Sprintf("tmdb:%d:%s:s%d:e%d", item.TMDBID, mediaType, item.SeasonNumber, item.EpisodeNumber)
		}
		return fmt.Sprintf("tmdb:%d:%s", item.TMDBID, mediaType)
	}
	title := normalizeLogicalTitle(item.Title)
	if title == "" {
		return ""
	}
	return fmt.Sprintf("title:%s:%d:%s:s%d:e%d", title, item.Year, mediaType, item.SeasonNumber, item.EpisodeNumber)
}

func normalizeLogicalTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	space := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func itemPresentationScore(item Item) int64 {
	var score int64
	if item.PosterURL != "" {
		score += 1000
	}
	if item.BackdropURL != "" {
		score += 700
	}
	if item.LogoURL != "" {
		score += 300
	}
	if item.Overview != "" {
		score += 200
	}
	if item.MetadataStatus != "" && item.MetadataStatus != "pending" && item.MetadataStatus != "error" {
		score += 500
	}
	if item.Rating > 0 {
		score += 100
	}
	// Prefer the oldest stable row when presentation data is equivalent. This
	// normally corresponds to Origem 1 and keeps card IDs stable across scans.
	score -= item.ID
	return score
}
