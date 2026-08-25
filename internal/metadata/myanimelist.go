package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MyAnimeListProvider struct {
	client *http.Client
	mu     sync.Mutex
	last   time.Time
}

func NewMyAnimeListProvider() *MyAnimeListProvider {
	return &MyAnimeListProvider{client: &http.Client{Timeout: 22 * time.Second}}
}

func (p *MyAnimeListProvider) Name() string { return "myanimelist" }
func (p *MyAnimeListProvider) Ready() bool  { return true }
func (p *MyAnimeListProvider) Supports(kind string) bool {
	return kind == "anime" || kind == "mixed"
}

type jikanAnime struct {
	MALID         int64   `json:"mal_id"`
	Title         string  `json:"title"`
	TitleEnglish  string  `json:"title_english"`
	TitleJapanese string  `json:"title_japanese"`
	Type          string  `json:"type"`
	Synopsis      string  `json:"synopsis"`
	Score         float64 `json:"score"`
	Year          int     `json:"year"`
	Duration      string  `json:"duration"`
	Aired         struct {
		From string `json:"from"`
	} `json:"aired"`
	Images struct {
		JPG struct {
			ImageURL      string `json:"image_url"`
			LargeImageURL string `json:"large_image_url"`
		} `json:"jpg"`
		WebP struct {
			ImageURL      string `json:"image_url"`
			LargeImageURL string `json:"large_image_url"`
		} `json:"webp"`
	} `json:"images"`
	Trailer struct {
		URL string `json:"url"`
	} `json:"trailer"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Themes []struct {
		Name string `json:"name"`
	} `json:"themes"`
}

type jikanSearchResponse struct {
	Data []jikanAnime `json:"data"`
}

func (p *MyAnimeListProvider) Lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if strings.TrimSpace(parsed.Title) == "" {
		return Result{}, errors.New("MyAnimeList: empty title")
	}
	var attempts []string
	for _, title := range parsed.SearchTitles() {
		attempts = append(attempts, title)
		anime, err := p.search(ctx, title, parsed.Year)
		if err != nil {
			return Result{}, err
		}
		if anime.MALID != 0 {
			return malResult(anime, parsed), nil
		}
	}
	return Result{}, fmt.Errorf("MyAnimeList: no match; tried %s", strings.Join(attempts, " | "))
}

func (p *MyAnimeListProvider) search(ctx context.Context, title string, year int) (jikanAnime, error) {
	// Public Jikan endpoints are rate limited. Serialize StormFlix requests and
	// keep a small interval so a bulk metadata retry does not hammer the API.
	p.mu.Lock()
	wait := 380*time.Millisecond - time.Since(p.last)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			p.mu.Unlock()
			return jikanAnime{}, ctx.Err()
		}
	}
	p.last = time.Now()
	p.mu.Unlock()

	q := url.Values{}
	q.Set("q", title)
	q.Set("limit", "10")
	q.Set("sfw", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.jikan.moe/v4/anime?"+q.Encode(), nil)
	if err != nil {
		return jikanAnime{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.6 MyAnimeList metadata agent")
	resp, err := p.client.Do(req)
	if err != nil {
		return jikanAnime{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return jikanAnime{}, errors.New("MyAnimeList/Jikan: rate limit; tente reprocessar em alguns segundos")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return jikanAnime{}, fmt.Errorf("MyAnimeList/Jikan: HTTP %d", resp.StatusCode)
	}
	var data jikanSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return jikanAnime{}, err
	}
	if len(data.Data) == 0 {
		return jikanAnime{}, nil
	}

	wanted := normalizeTitle(title)
	bestScore := -1.0
	var best jikanAnime
	for _, candidate := range data.Data {
		score := candidate.Score / 100
		for _, name := range []string{candidate.Title, candidate.TitleEnglish, candidate.TitleJapanese} {
			n := normalizeTitle(name)
			if n != "" && n == wanted {
				score += 12
			} else if n != "" && wanted != "" && (strings.Contains(n, wanted) || strings.Contains(wanted, n)) {
				score += 3
			}
		}
		candidateYear := candidate.Year
		if candidateYear == 0 && len(candidate.Aired.From) >= 4 {
			candidateYear, _ = strconv.Atoi(candidate.Aired.From[:4])
		}
		if year > 0 && candidateYear == year {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best, nil
}

func malResult(anime jikanAnime, parsed ParsedName) Result {
	title := strings.TrimSpace(anime.TitleEnglish)
	if title == "" {
		title = strings.TrimSpace(anime.Title)
	}
	year := anime.Year
	if year == 0 && len(anime.Aired.From) >= 4 {
		year, _ = strconv.Atoi(anime.Aired.From[:4])
	}
	genres := make([]string, 0, len(anime.Genres)+len(anime.Themes))
	for _, g := range anime.Genres {
		if strings.TrimSpace(g.Name) != "" {
			genres = append(genres, g.Name)
		}
	}
	for _, g := range anime.Themes {
		if strings.TrimSpace(g.Name) != "" {
			genres = append(genres, g.Name)
		}
	}
	result := Result{
		Title: title, OriginalTitle: anime.TitleJapanese, MediaType: "anime", Year: year,
		Season: parsed.Season, Episode: parsed.Episode, Overview: anime.Synopsis, Genres: genres,
		Rating: anime.Score, RuntimeMinutes: parseMALDuration(anime.Duration), Provider: "myanimelist",
		ProviderID: strconv.FormatInt(anime.MALID, 10), MALID: anime.MALID, TrailerURL: anime.Trailer.URL,
	}
	poster := strings.TrimSpace(anime.Images.WebP.LargeImageURL)
	if poster == "" {
		poster = strings.TrimSpace(anime.Images.JPG.LargeImageURL)
	}
	if poster == "" {
		poster = strings.TrimSpace(anime.Images.JPG.ImageURL)
	}
	if poster != "" {
		result.Artwork = append(result.Artwork, Artwork{Kind: "poster", Provider: "myanimelist", URL: poster, Score: 125})
	}
	return result
}

func parseMALDuration(value string) int {
	value = strings.ToLower(value)
	minutes := 0
	fields := strings.Fields(value)
	for i, field := range fields {
		n, err := strconv.Atoi(strings.Trim(field, ",."))
		if err != nil || i+1 >= len(fields) {
			continue
		}
		next := fields[i+1]
		if strings.HasPrefix(next, "hr") {
			minutes += n * 60
		} else if strings.HasPrefix(next, "min") {
			minutes += n
		}
	}
	return minutes
}
