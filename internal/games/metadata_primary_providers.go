package games

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var igdbPlatform = map[string]int{"nes": 18, "snes": 19, "genesis": 29, "gb": 33, "gbc": 22, "gba": 24}

func fetchIGDB(ctx context.Context, clientID, clientSecret string, game metadataGameRow) (*gameMetadataCandidate, error) {
	tokenURL := "https://id.twitch.tv/oauth2/token?client_id=" + url.QueryEscape(clientID) + "&client_secret=" + url.QueryEscape(clientSecret) + "&grant_type=client_credentials"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OAuth HTTP %d", resp.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return nil, errors.New("OAuth token inválido")
	}

	platformID := igdbPlatform[game.Platform]
	query := fmt.Sprintf(`fields id,name,summary,first_release_date,rating,genres.name,involved_companies.company.name,involved_companies.developer,involved_companies.publisher,cover.url,screenshots.url; search %q;`, game.Title)
	if platformID > 0 {
		query += fmt.Sprintf(" where platforms = (%d);", platformID)
	}
	query += " limit 10;"
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.igdb.com/v4/games", bytes.NewBufferString(query))
	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "text/plain")
	resp, err = (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("IGDB HTTP %d", resp.StatusCode)
	}

	type named struct {
		Name string `json:"name"`
	}
	type companyLink struct {
		Developer bool  `json:"developer"`
		Publisher bool  `json:"publisher"`
		Company   named `json:"company"`
	}
	type image struct {
		URL string `json:"url"`
	}
	var items []struct {
		ID                int64         `json:"id"`
		Name              string        `json:"name"`
		Summary           string        `json:"summary"`
		FirstReleaseDate  int64         `json:"first_release_date"`
		Rating            float64       `json:"rating"`
		Genres            []named       `json:"genres"`
		InvolvedCompanies []companyLink `json:"involved_companies"`
		Cover             image         `json:"cover"`
		Screenshots       []image       `json:"screenshots"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&items); err != nil {
		return nil, err
	}
	best := -1
	bestScore := 0.0
	for i := range items {
		score := titleScore(game.Title, items[i].Name)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 || bestScore < .62 {
		return nil, nil
	}
	item := items[best]
	out := &gameMetadataCandidate{Provider: "igdb", ProviderID: strconv.FormatInt(item.ID, 10), Title: item.Name, Overview: item.Summary, Rating: item.Rating}
	if item.FirstReleaseDate > 0 {
		out.ReleaseYear = time.Unix(item.FirstReleaseDate, 0).UTC().Year()
	}
	for _, genre := range item.Genres {
		if genre.Name != "" {
			out.Genres = append(out.Genres, genre.Name)
		}
	}
	for _, link := range item.InvolvedCompanies {
		if link.Developer && link.Company.Name != "" {
			out.Developers = append(out.Developers, link.Company.Name)
		}
		if link.Publisher && link.Company.Name != "" {
			out.Publishers = append(out.Publishers, link.Company.Name)
		}
	}
	if item.Cover.URL != "" {
		out.CoverURL = igdbImage(item.Cover.URL, "t_cover_big_2x")
	}
	for _, shot := range item.Screenshots {
		if shot.URL != "" {
			out.Screenshots = append(out.Screenshots, igdbImage(shot.URL, "t_screenshot_big"))
		}
	}
	return out, nil
}

func igdbImage(raw, size string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	return strings.Replace(raw, "t_thumb", size, 1)
}

func fetchMobyGames(ctx context.Context, apiKey string, game metadataGameRow) (*gameMetadataCandidate, error) {
	endpoint := "https://api.mobygames.com/v1/games?format=normal&limit=10&title=" + url.QueryEscape(game.Title) + "&api_key=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "StormFlix/0.25 Games")
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Games []struct {
			GameID      int64   `json:"game_id"`
			Title       string  `json:"title"`
			Description string  `json:"description"`
			MobyScore   float64 `json:"moby_score"`
			Genres      []struct {
				Name string `json:"genre_name"`
			} `json:"genres"`
			Platforms []struct {
				Date string `json:"first_release_date"`
				Name string `json:"platform_name"`
			} `json:"platforms"`
			SampleCover struct {
				Image string `json:"image"`
			} `json:"sample_cover"`
			Screenshots []struct {
				Image string `json:"image"`
			} `json:"sample_screenshots"`
		} `json:"games"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&result); err != nil {
		return nil, err
	}
	best := -1
	bestScore := 0.0
	for i := range result.Games {
		score := titleScore(game.Title, result.Games[i].Title)
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 || bestScore < .68 {
		return nil, nil
	}
	item := result.Games[best]
	out := &gameMetadataCandidate{Provider: "mobygames", ProviderID: strconv.FormatInt(item.GameID, 10), Title: item.Title, Overview: item.Description, Rating: item.MobyScore, CoverURL: item.SampleCover.Image}
	for _, genre := range item.Genres {
		if genre.Name != "" {
			out.Genres = append(out.Genres, genre.Name)
		}
	}
	for _, platform := range item.Platforms {
		if len(platform.Date) >= 4 {
			if year, err := strconv.Atoi(platform.Date[:4]); err == nil && (out.ReleaseYear == 0 || year < out.ReleaseYear) {
				out.ReleaseYear = year
			}
		}
	}
	for _, shot := range item.Screenshots {
		if shot.Image != "" {
			out.Screenshots = append(out.Screenshots, shot.Image)
		}
	}
	return out, nil
}

func fetchSteamGridDB(ctx context.Context, apiKey, title string) (int64, string, error) {
	client := &http.Client{Timeout: 25 * time.Second}
	endpoint := "https://steamgriddb.com/api/v2/search/autocomplete/" + url.PathEscape(title)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "StormFlix/0.25 Games")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", fmt.Errorf("search HTTP %d", resp.StatusCode)
	}
	var search struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&search); err != nil {
		return 0, "", err
	}
	bestID := int64(0)
	bestScore := 0.0
	for _, item := range search.Data {
		score := titleScore(title, item.Name)
		if score > bestScore {
			bestScore, bestID = score, item.ID
		}
	}
	if bestID == 0 || bestScore < .64 {
		return 0, "", nil
	}

	endpoint = fmt.Sprintf("https://steamgriddb.com/api/v2/grids/game/%d?types=static&nsfw=false&humor=false&limit=50", bestID)
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "StormFlix/0.25 Games")
	resp, err = client.Do(req)
	if err != nil {
		return bestID, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bestID, "", fmt.Errorf("grid HTTP %d", resp.StatusCode)
	}
	var grids struct {
		Data []struct {
			URL    string  `json:"url"`
			Score  float64 `json:"score"`
			Width  int     `json:"width"`
			Height int     `json:"height"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&grids); err != nil {
		return bestID, "", err
	}
	sort.SliceStable(grids.Data, func(i, j int) bool {
		portraitI := grids.Data[i].Height > grids.Data[i].Width
		portraitJ := grids.Data[j].Height > grids.Data[j].Width
		if portraitI != portraitJ {
			return portraitI
		}
		return grids.Data[i].Score > grids.Data[j].Score
	})
	for _, grid := range grids.Data {
		if grid.URL != "" && grid.Height > grid.Width {
			return bestID, grid.URL, nil
		}
	}
	return bestID, "", nil
}

func titleScore(a, b string) float64 {
	a, b = normalizeGameTitle(a), normalizeGameTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter, longer := len(a), len(b)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return .75 + .2*float64(shorter)/float64(longer)
	}
	setA, setB := map[string]bool{}, map[string]bool{}
	for _, token := range strings.Fields(a) {
		setA[token] = true
	}
	for _, token := range strings.Fields(b) {
		setB[token] = true
	}
	intersection := 0
	for token := range setA {
		if setB[token] {
			intersection++
		}
	}
	if intersection == 0 {
		return 0
	}
	precision := float64(intersection) / float64(len(setA))
	recall := float64(intersection) / float64(len(setB))
	return 2 * precision * recall / (precision + recall)
}

func normalizeGameTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
		"&", " and ", "_", " ", "-", " ", ":", " ", ".", " ", ",", " ", "'", "", "’", "",
	).Replace(value)
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			out = append(out, r)
		}
	}
	return strings.Join(strings.Fields(string(out)), " ")
}
