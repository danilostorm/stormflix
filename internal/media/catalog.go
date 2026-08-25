package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

type Person struct {
	Name       string `json:"name"`
	Character  string `json:"character,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
}

type Detail struct {
	Item
	OriginalTitle     string   `json:"original_title"`
	Tagline           string   `json:"tagline"`
	Directors         []string `json:"directors"`
	Cast              []Person `json:"cast"`
	TrailerURL        string   `json:"trailer_url"`
	ThemePreviewURL   string   `json:"theme_preview_url"`
	ThemePreviewTitle string   `json:"theme_preview_title"`
	IMDbID            string   `json:"imdb_id"`
	TMDBID            int64    `json:"tmdb_id"`
	Related           []Item   `json:"related"`
}

type HomeRow struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Items []Item `json:"items"`
}

type HomeFeed struct {
	Hero                 *Item     `json:"hero,omitempty"`
	Rows                 []HomeRow `json:"rows"`
	ServerName           string    `json:"server_name"`
	ThemePreviewEnabled  bool      `json:"theme_preview_enabled"`
	ThemePreviewVolume   int       `json:"theme_preview_volume"`
	ThemePreviewAutoplay bool      `json:"theme_preview_autoplay"`
}

func (s *Service) Home(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	items, err := s.List(ctx, 0, "", 500, 0, allowedLibraryIDs)
	if err != nil {
		return HomeFeed{}, err
	}
	feed := HomeFeed{Rows: []HomeRow{}, ServerName: serverName, ThemePreviewEnabled: themeEnabled, ThemePreviewVolume: themeVolume, ThemePreviewAutoplay: themeAutoplay}
	if len(items) == 0 {
		return feed, nil
	}

	recent := append([]Item(nil), items...)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].ModifiedUnix > recent[j].ModifiedUnix })
	recent = capItems(recent, 24)
	feed.Rows = append(feed.Rows, HomeRow{ID: "recent", Title: "Adicionados recentemente", Items: recent})

	top := append([]Item(nil), items...)
	sort.SliceStable(top, func(i, j int) bool {
		if top[i].Rating == top[j].Rating {
			return top[i].ModifiedUnix > top[j].ModifiedUnix
		}
		return top[i].Rating > top[j].Rating
	})
	topRated := make([]Item, 0, 24)
	for _, item := range top {
		if item.Rating <= 0 {
			continue
		}
		topRated = append(topRated, item)
		if len(topRated) == 24 {
			break
		}
	}
	if len(topRated) > 0 {
		feed.Rows = append(feed.Rows, HomeRow{ID: "top-rated", Title: "Mais bem avaliados", Items: topRated})
	}

	libraryOrder := []string{}
	seenLibraries := map[string]bool{}
	byLibrary := map[string][]Item{}
	for _, item := range recent {
		name := strings.TrimSpace(item.LibraryName)
		if name == "" || seenLibraries[name] {
			continue
		}
		seenLibraries[name] = true
		libraryOrder = append(libraryOrder, name)
	}
	for _, item := range items {
		name := strings.TrimSpace(item.LibraryName)
		if name == "" {
			continue
		}
		if !seenLibraries[name] {
			seenLibraries[name] = true
			libraryOrder = append(libraryOrder, name)
		}
		if len(byLibrary[name]) < 24 {
			byLibrary[name] = append(byLibrary[name], item)
		}
	}
	for _, name := range libraryOrder {
		if len(byLibrary[name]) == 0 {
			continue
		}
		feed.Rows = append(feed.Rows, HomeRow{ID: "library-" + slug(name), Title: name, Items: byLibrary[name]})
	}

	genreMap := map[string][]Item{}
	genreCount := map[string]int{}
	for _, item := range items {
		for _, genre := range item.Genres {
			genre = strings.TrimSpace(genre)
			if genre == "" {
				continue
			}
			genreCount[genre]++
			if len(genreMap[genre]) < 24 {
				genreMap[genre] = append(genreMap[genre], item)
			}
		}
	}
	genres := make([]string, 0, len(genreCount))
	for genre := range genreCount {
		genres = append(genres, genre)
	}
	sort.Slice(genres, func(i, j int) bool { return genreCount[genres[i]] > genreCount[genres[j]] })
	if len(genres) > 8 {
		genres = genres[:8]
	}
	for _, genre := range genres {
		if len(genreMap[genre]) >= 2 {
			feed.Rows = append(feed.Rows, HomeRow{ID: "genre-" + slug(genre), Title: genre, Items: genreMap[genre]})
		}
	}

	heroPool := recent
	if heroMode == "top_rated" && len(topRated) > 0 {
		heroPool = topRated
	}
	if heroMode == "featured" {
		for _, item := range top {
			if item.BackdropURL != "" {
				feed.Hero = copyItem(item)
				break
			}
		}
	}
	if feed.Hero == nil && len(heroPool) > 0 {
		index := 0
		if heroMode == "random" && len(heroPool) > 1 {
			index = int(time.Now().Unix()/3600) % len(heroPool)
		}
		feed.Hero = copyItem(heroPool[index])
	}
	return feed, nil
}

func (s *Service) Detail(ctx context.Context, id int64, allowedLibraryIDs []int64) (Detail, error) {
	var d Detail
	var genresJSON, castJSON, directorsJSON string
	err := s.db.QueryRowContext(ctx, `SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE(mm.original_title,''),COALESCE(mm.tagline,''),COALESCE(mm.directors_json,'[]'),COALESCE(mm.cast_json,'[]'),COALESCE(mm.trailer_url,''),COALESCE(mm.theme_preview_url,''),COALESCE(mm.theme_preview_title,''),COALESCE(mm.imdb_id,''),COALESCE(mm.tmdb_id,0)
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=? AND m.available=1`, id).
		Scan(&d.ID, &d.LibraryID, &d.LibraryName, &d.Title, &d.Extension, &d.SizeBytes, &d.ModifiedUnix, &d.Available,
			&d.MediaType, &d.Year, &d.SeasonNumber, &d.EpisodeNumber, &d.Overview, &genresJSON, &d.Rating, &d.RuntimeMinutes, &d.MetadataStatus,
			&d.PosterURL, &d.BackdropURL, &d.LogoURL, &d.OriginalTitle, &d.Tagline, &directorsJSON, &castJSON, &d.TrailerURL, &d.ThemePreviewURL, &d.ThemePreviewTitle, &d.IMDbID, &d.TMDBID)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, sql.ErrNoRows
	}
	if err != nil {
		return Detail{}, err
	}
	if allowedLibraryIDs != nil && !ContainsLibrary(allowedLibraryIDs, d.LibraryID) {
		return Detail{}, errors.New("library access denied")
	}
	_ = json.Unmarshal([]byte(genresJSON), &d.Genres)
	_ = json.Unmarshal([]byte(castJSON), &d.Cast)
	_ = json.Unmarshal([]byte(directorsJSON), &d.Directors)

	candidates, err := s.List(ctx, 0, "", 500, 0, allowedLibraryIDs)
	if err == nil {
		type scored struct {
			item  Item
			score float64
		}
		scoredItems := []scored{}
		wantedGenres := map[string]bool{}
		for _, genre := range d.Genres {
			wantedGenres[strings.ToLower(genre)] = true
		}
		for _, candidate := range candidates {
			if candidate.ID == d.ID {
				continue
			}
			score := candidate.Rating / 10
			if candidate.MediaType == d.MediaType && d.MediaType != "" {
				score += 2
			}
			for _, genre := range candidate.Genres {
				if wantedGenres[strings.ToLower(genre)] {
					score += 5
				}
			}
			if score > 0 {
				scoredItems = append(scoredItems, scored{item: candidate, score: score})
			}
		}
		sort.SliceStable(scoredItems, func(i, j int) bool { return scoredItems[i].score > scoredItems[j].score })
		for _, candidate := range scoredItems {
			d.Related = append(d.Related, candidate.item)
			if len(d.Related) == 18 {
				break
			}
		}
	}
	if d.Related == nil {
		d.Related = []Item{}
	}
	return d, nil
}

func capItems(items []Item, limit int) []Item {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func copyItem(item Item) *Item {
	copy := item
	return &copy
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
