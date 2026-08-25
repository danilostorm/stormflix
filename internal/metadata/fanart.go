package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FanartProvider struct {
	apiKey    string
	clientKey string
	client    *http.Client
}

func NewFanartProvider(apiKey, clientKey string) *FanartProvider {
	return &FanartProvider{
		apiKey:    strings.TrimSpace(apiKey),
		clientKey: strings.TrimSpace(clientKey),
		client:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *FanartProvider) Ready() bool { return p.apiKey != "" || p.clientKey != "" }

func (p *FanartProvider) Enrich(ctx context.Context, result *Result) error {
	if !p.Ready() || result == nil {
		return nil
	}
	var endpoint string
	if result.MediaType == "movie" && result.TMDBID > 0 {
		endpoint = fmt.Sprintf("https://webservice.fanart.tv/v3.2/movies/%d", result.TMDBID)
	} else if (result.MediaType == "series" || result.MediaType == "anime") && result.TVDBID > 0 {
		endpoint = fmt.Sprintf("https://webservice.fanart.tv/v3.2/tv/%d", result.TVDBID)
	} else {
		return nil
	}
	u, _ := url.Parse(endpoint)
	q := u.Query()
	if p.apiKey != "" {
		q.Set("api_key", p.apiKey)
	}
	if p.clientKey != "" {
		q.Set("client_key", p.clientKey)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.3")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Fanart.tv: HTTP %d", resp.StatusCode)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}

	types := map[string]struct {
		Kind  string
		Score float64
	}{
		"movieposter":     {"poster", 90},
		"tvposter":        {"poster", 90},
		"moviebackground": {"backdrop", 95},
		"showbackground":  {"backdrop", 95},
		"hdmovielogo":     {"logo", 130},
		"movielogo":       {"logo", 125},
		"hdtvlogo":        {"logo", 130},
		"clearlogo":       {"logo", 125},
		"movieart":        {"clearart", 100},
		"clearart":        {"clearart", 100},
		"hdclearart":      {"clearart", 105},
		"moviethumb":      {"thumb", 90},
		"tvthumb":         {"thumb", 90},
	}
	for key, spec := range types {
		data, ok := raw[key]
		if !ok {
			continue
		}
		var images []struct {
			URL   string `json:"url"`
			Lang  string `json:"lang"`
			Likes string `json:"likes"`
		}
		if err := json.Unmarshal(data, &images); err != nil {
			continue
		}
		for _, image := range images {
			if image.URL == "" {
				continue
			}
			likes, _ := strconv.Atoi(image.Likes)
			result.Artwork = append(result.Artwork, Artwork{
				Kind:     spec.Kind,
				Provider: "fanart.tv",
				URL:      image.URL,
				Language: image.Lang,
				Score:    spec.Score + float64(likes)/100,
			})
		}
	}
	return nil
}
