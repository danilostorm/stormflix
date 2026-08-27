package metadata

import (
	"bytes"
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

const tvdbBaseURL = "https://api4.thetvdb.com/v4"

type TVDBProvider struct {
	apiKey   string
	pin      string
	language string
	client   *http.Client
	mu       sync.Mutex
	token    string
	tokenAt  time.Time
}

func NewTVDBProvider(apiKey, pin, language string) *TVDBProvider {
	return &TVDBProvider{
		apiKey: strings.TrimSpace(apiKey),
		pin: strings.TrimSpace(pin),
		language: strings.TrimSpace(language),
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *TVDBProvider) Name() string { return "tvdb" }
func (p *TVDBProvider) Ready() bool { return p != nil && p.apiKey != "" }
func (p *TVDBProvider) Supports(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "series", "animation_series", "anime_series", "anime", "mixed":
		return true
	default:
		return false
	}
}

type tvdbEnvelope[T any] struct {
	Status string `json:"status"`
	Data T `json:"data"`
}

type tvdbSearchItem struct {
	TVDBID string `json:"tvdb_id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Year string `json:"year"`
	FirstAirTime string `json:"first_air_time"`
	ImageURL string `json:"image_url"`
	Aliases []string `json:"aliases"`
	Translations map[string]string `json:"translations"`
	Overviews map[string]string `json:"overviews"`
}

type tvdbSeriesData struct {
	ID int64 `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Image string `json:"image"`
	FirstAired string `json:"firstAired"`
	Overview string `json:"overview"`
	AverageRuntime int `json:"averageRuntime"`
	Genres []struct { Name string `json:"name"` } `json:"genres"`
	RemoteIDs []struct {
		ID string `json:"id"`
		SourceName string `json:"sourceName"`
		Type int `json:"type"`
	} `json:"remoteIds"`
	Artworks []struct {
		Image string `json:"image"`
		Thumbnail string `json:"thumbnail"`
		Language string `json:"language"`
		Score float64 `json:"score"`
		Type int `json:"type"`
	} `json:"artworks"`
}

type tvdbEpisode struct {
	ID int64 `json:"id"`
	Name string `json:"name"`
	Overview string `json:"overview"`
	Aired string `json:"aired"`
	Runtime int `json:"runtime"`
	Image string `json:"image"`
	SeasonNumber int `json:"seasonNumber"`
	Number int `json:"number"`
}

type tvdbSeriesEpisodesData struct {
	Series tvdbSeriesData `json:"series"`
	Episodes []tvdbEpisode `json:"episodes"`
}

func (p *TVDBProvider) Lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if !p.Ready() {
		return Result{}, errors.New("TheTVDB is not configured")
	}
	if !p.Supports(item.LibraryKind) {
		return Result{}, errors.New("TheTVDB does not support this library type")
	}
	var lastErr error = errors.New("TheTVDB: no match")
	for _, title := range parsed.SearchTitles() {
		candidate, err := p.searchSeries(ctx, title, parsed.Year)
		if err != nil {
			lastErr = err
			continue
		}
		if candidate.TVDBID == "" {
			continue
		}
		id, _ := strconv.ParseInt(candidate.TVDBID, 10, 64)
		if id <= 0 {
			continue
		}
		result, err := p.seriesResult(ctx, id, candidate, parsed, item)
		if err != nil {
			lastErr = err
			continue
		}
		return result, nil
	}
	return Result{}, lastErr
}

func (p *TVDBProvider) searchSeries(ctx context.Context, title string, year int) (tvdbSearchItem, error) {
	q := url.Values{}
	q.Set("query", title)
	q.Set("type", "series")
	q.Set("limit", "20")
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	var response tvdbEnvelope[[]tvdbSearchItem]
	if err := p.get(ctx, tvdbBaseURL+"/search?"+q.Encode(), &response); err != nil {
		return tvdbSearchItem{}, err
	}
	if len(response.Data) == 0 && year > 0 {
		q.Del("year")
		if err := p.get(ctx, tvdbBaseURL+"/search?"+q.Encode(), &response); err != nil {
			return tvdbSearchItem{}, err
		}
	}
	if len(response.Data) == 0 {
		return tvdbSearchItem{}, nil
	}
	want := normalizeTitle(title)
	bestScore := -1.0
	var best tvdbSearchItem
	for _, candidate := range response.Data {
		if candidate.TVDBID == "" || (!strings.EqualFold(candidate.Type, "series") && candidate.Type != "") {
			continue
		}
		names := []string{candidate.Name}
		names = append(names, candidate.Aliases...)
		for _, translated := range candidate.Translations {
			names = append(names, translated)
		}
		score := titleMatchScore(want, names)
		candidateYear := yearFromDate(firstNonEmpty(candidate.FirstAirTime, candidate.Year))
		if year > 0 && candidateYear == year {
			score += 3
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	// Reject unrelated first hits. Search providers must not be allowed to turn
	// a release filename into an arbitrary series just because something ranked.
	if bestScore < 4 {
		return tvdbSearchItem{}, nil
	}
	return best, nil
}

func titleMatchScore(want string, names []string) float64 {
	if want == "" {
		return 0
	}
	best := 0.0
	wantTokens := strings.Fields(want)
	for _, name := range names {
		normalized := normalizeTitle(name)
		if normalized == "" {
			continue
		}
		score := 0.0
		if normalized == want {
			score = 12
		} else if strings.Contains(normalized, want) || strings.Contains(want, normalized) {
			score = 7
		} else {
			matched := 0
			for _, token := range wantTokens {
				if len(token) >= 3 && strings.Contains(normalized, token) {
					matched++
				}
			}
			if len(wantTokens) > 0 {
				score = 6 * float64(matched) / float64(len(wantTokens))
			}
		}
		if score > best {
			best = score
		}
	}
	return best
}

func (p *TVDBProvider) seriesResult(ctx context.Context, id int64, search tvdbSearchItem, parsed ParsedName, item SourceItem) (Result, error) {
	var response tvdbEnvelope[tvdbSeriesData]
	if err := p.get(ctx, fmt.Sprintf("%s/series/%d/extended?meta=translations", tvdbBaseURL, id), &response); err != nil {
		return Result{}, err
	}
	series := response.Data
	if series.ID == 0 {
		series.ID = id
	}
	result := Result{
		Title: firstNonEmpty(series.Name, search.Name, parsed.Title),
		MediaType: "series",
		Year: yearFromDate(firstNonEmpty(series.FirstAired, search.FirstAirTime, search.Year)),
		Overview: firstNonEmpty(series.Overview, preferredTVDBText(search.Overviews, p.language)),
		Genres: tvdbGenreNames(series.Genres),
		RuntimeMinutes: series.AverageRuntime,
		Provider: "tvdb",
		ProviderID: strconv.FormatInt(series.ID, 10),
		TVDBID: series.ID,
		Season: parsed.Season,
		Episode: parsed.Episode,
	}
	for _, remote := range series.RemoteIDs {
		source := strings.ToLower(remote.SourceName)
		switch {
		case strings.Contains(source, "imdb"):
			result.IMDbID = remote.ID
		case strings.Contains(source, "themoviedb") || strings.Contains(source, "tmdb"):
			if result.TMDBID == 0 {
				result.TMDBID, _ = strconv.ParseInt(remote.ID, 10, 64)
			}
		}
	}
	if image := firstNonEmpty(series.Image, search.ImageURL); image != "" {
		result.Artwork = append(result.Artwork, Artwork{Kind: "poster", Provider: "tvdb", URL: normalizeTVDBImage(image), Score: 100})
	}
	for _, art := range series.Artworks {
		image := firstNonEmpty(art.Image, art.Thumbnail)
		if image == "" {
			continue
		}
		kind := "backdrop"
		// TVDB artwork type IDs can vary as the API evolves. Keep the series
		// primary image as poster and treat extended artwork as enrichment.
		result.Artwork = append(result.Artwork, Artwork{Kind: kind, Provider: "tvdb", URL: normalizeTVDBImage(image), Language: art.Language, Score: 70 + art.Score})
	}

	if parsed.Episode > 0 {
		if episode, err := p.findEpisode(ctx, id, parsed, item); err == nil && episode.ID > 0 {
			if episode.Name != "" {
				result.Title = fmt.Sprintf("%s S%02dE%02d · %s", firstNonEmpty(series.Name, search.Name, parsed.Title), maxTVDBInt(parsed.Season, 1), parsed.Episode, episode.Name)
			}
			if episode.Overview != "" {
				result.Overview = episode.Overview
			}
			if episode.Runtime > 0 {
				result.RuntimeMinutes = episode.Runtime
			}
			if episode.Image != "" {
				result.Artwork = append(result.Artwork, Artwork{Kind: "backdrop", Provider: "tvdb", URL: normalizeTVDBImage(episode.Image), Score: 130})
			}
		}
	}
	return result, nil
}

func (p *TVDBProvider) findEpisode(ctx context.Context, seriesID int64, parsed ParsedName, item SourceItem) (tvdbEpisode, error) {
	season := maxTVDBInt(parsed.Season, 1)
	orders := []string{"official"}
	if item.LibraryKind == "anime" || item.LibraryKind == "anime_series" || item.LibraryKind == "animation_series" {
		orders = append(orders, "absolute")
	}
	for _, order := range orders {
		q := url.Values{}
		q.Set("season", strconv.Itoa(season))
		q.Set("episodeNumber", strconv.Itoa(parsed.Episode))
		var response tvdbEnvelope[tvdbSeriesEpisodesData]
		if err := p.get(ctx, fmt.Sprintf("%s/series/%d/episodes/%s?%s", tvdbBaseURL, seriesID, order, q.Encode()), &response); err != nil {
			continue
		}
		if len(response.Data.Episodes) > 0 {
			return response.Data.Episodes[0], nil
		}
	}
	return tvdbEpisode{}, errors.New("TheTVDB: episode not found")
}

func (p *TVDBProvider) tokenFor(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Since(p.tokenAt) < 28*24*time.Hour {
		return p.token, nil
	}
	body := map[string]string{"apikey": p.apiKey}
	if p.pin != "" {
		body["pin"] = p.pin
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tvdbBaseURL+"/login", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.17")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("TheTVDB login: HTTP %d", resp.StatusCode)
	}
	var envelope tvdbEnvelope[struct { Token string `json:"token"` }]
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	if envelope.Data.Token == "" {
		return "", errors.New("TheTVDB login returned no token")
	}
	p.token = envelope.Data.Token
	p.tokenAt = time.Now()
	return p.token, nil
}

func (p *TVDBProvider) get(ctx context.Context, rawURL string, dest any) error {
	token, err := p.tokenFor(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.17")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		p.mu.Lock()
		p.token = ""
		p.tokenAt = time.Time{}
		p.mu.Unlock()
		return errors.New("TheTVDB token rejected")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TheTVDB: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func preferredTVDBText(values map[string]string, language string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range tvdbLanguageCandidates(language) {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func tvdbLanguageCandidates(language string) []string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "pt-br", "pt", "pt-pt":
		return []string{"por", "pt", "eng"}
	case "es", "es-es", "es-mx":
		return []string{"spa", "es", "eng"}
	default:
		return []string{"eng", "en"}
	}
}

func tvdbGenreNames(items []struct { Name string `json:"name"` }) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name)
		}
	}
	return out
}

func normalizeTVDBImage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return "https://artworks.thetvdb.com" + value
}

func maxTVDBInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
