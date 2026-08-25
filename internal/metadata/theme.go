package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ThemeProvider struct {
	country string
	client  *http.Client
}

func NewThemeProvider(country string) *ThemeProvider {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		country = "BR"
	}
	return &ThemeProvider{country: country, client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *ThemeProvider) Lookup(ctx context.Context, title string, year int) (previewURL, previewTitle string, err error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "", nil
	}
	term := title + " soundtrack"
	if year > 0 {
		term += fmt.Sprintf(" %d", year)
	}
	q := url.Values{}
	q.Set("term", term)
	q.Set("media", "music")
	q.Set("entity", "song")
	q.Set("limit", "12")
	q.Set("country", p.country)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://itunes.apple.com/search?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.4")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("soundtrack preview: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Results []struct {
			TrackName      string `json:"trackName"`
			CollectionName string `json:"collectionName"`
			ArtistName     string `json:"artistName"`
			PreviewURL     string `json:"previewUrl"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}
	want := normalizeTitle(title)
	bestScore := -1
	bestURL, bestTitle := "", ""
	for _, item := range data.Results {
		if item.PreviewURL == "" {
			continue
		}
		haystack := strings.ToLower(item.TrackName + " " + item.CollectionName)
		score := 0
		if strings.Contains(normalizeTitle(item.CollectionName), want) {
			score += 8
		}
		if strings.Contains(haystack, "theme") || strings.Contains(haystack, "main title") || strings.Contains(haystack, "main theme") {
			score += 7
		}
		if strings.Contains(haystack, "soundtrack") || strings.Contains(haystack, "motion picture") {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			bestURL = item.PreviewURL
			bestTitle = strings.TrimSpace(item.TrackName + " · " + item.ArtistName)
		}
	}
	return bestURL, bestTitle, nil
}
