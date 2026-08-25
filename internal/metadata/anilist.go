package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type AniListProvider struct {
	client *http.Client
}

func NewAniListProvider() *AniListProvider {
	return &AniListProvider{client: &http.Client{Timeout: 20 * time.Second}}
}

func (p *AniListProvider) Name() string              { return "anilist" }
func (p *AniListProvider) Ready() bool               { return true }
func (p *AniListProvider) Supports(kind string) bool { return kind == "anime" }

func (p *AniListProvider) Lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	const query = `
query ($search: String!, $page: Int!) {
  Page(page: $page, perPage: 8) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH) {
      id
      idMal
      title { romaji english native }
      description(asHtml: false)
      genres
      averageScore
      episodes
      duration
      seasonYear
      coverImage { extraLarge large }
      bannerImage
    }
  }
}`
	payload := map[string]any{
		"query": query,
		"variables": map[string]any{"search": parsed.Title, "page": 1},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graphql.anilist.co", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.3")
	resp, err := p.client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("AniList: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Data struct {
			Page struct {
				Media []struct {
					ID          int64    `json:"id"`
					IDMal       int64    `json:"idMal"`
					Description string   `json:"description"`
					Genres      []string `json:"genres"`
					AverageScore float64 `json:"averageScore"`
					Episodes    int      `json:"episodes"`
					Duration    int      `json:"duration"`
					SeasonYear  int      `json:"seasonYear"`
					BannerImage string   `json:"bannerImage"`
					Title       struct {
						Romaji  string `json:"romaji"`
						English string `json:"english"`
						Native  string `json:"native"`
					} `json:"title"`
					CoverImage struct {
						ExtraLarge string `json:"extraLarge"`
						Large      string `json:"large"`
					} `json:"coverImage"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Result{}, err
	}
	if len(data.Data.Page.Media) == 0 {
		return Result{}, errors.New("AniList: no match")
	}

	want := normalizeTitle(parsed.Title)
	best := 0
	bestScore := -1.0
	for i, candidate := range data.Data.Page.Media {
		score := 0.0
		for _, title := range []string{candidate.Title.English, candidate.Title.Romaji, candidate.Title.Native} {
			n := normalizeTitle(title)
			if n == want && n != "" {
				score += 10
			} else if strings.Contains(n, want) || strings.Contains(want, n) {
				score += 2
			}
		}
		if parsed.Year > 0 && candidate.SeasonYear == parsed.Year {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	anime := data.Data.Page.Media[best]
	title := firstNonEmpty(anime.Title.English, anime.Title.Romaji, anime.Title.Native, parsed.Title)
	result := Result{
		Title:          title,
		MediaType:      "anime",
		Year:           anime.SeasonYear,
		Season:         parsed.Season,
		Episode:        parsed.Episode,
		Overview:       cleanDescription(anime.Description),
		Genres:         anime.Genres,
		Rating:         anime.AverageScore / 10,
		RuntimeMinutes: anime.Duration,
		Provider:       "anilist",
		ProviderID:     fmt.Sprintf("%d", anime.ID),
		AniListID:      anime.ID,
		MALID:          anime.IDMal,
	}
	poster := firstNonEmpty(anime.CoverImage.ExtraLarge, anime.CoverImage.Large)
	if poster != "" {
		result.Artwork = append(result.Artwork, Artwork{Kind: "poster", Provider: "anilist", URL: poster, Score: 100})
	}
	if anime.BannerImage != "" {
		result.Artwork = append(result.Artwork, Artwork{Kind: "backdrop", Provider: "anilist", URL: anime.BannerImage, Score: 100})
	}
	return result, nil
}

func cleanDescription(value string) string {
	value = strings.ReplaceAll(value, "<br>", "\n")
	value = strings.ReplaceAll(value, "<br/>", "\n")
	value = strings.ReplaceAll(value, "<br />", "\n")
	value = strings.ReplaceAll(value, "<i>", "")
	value = strings.ReplaceAll(value, "</i>", "")
	value = strings.ReplaceAll(value, "<b>", "")
	value = strings.ReplaceAll(value, "</b>", "")
	return strings.TrimSpace(value)
}
