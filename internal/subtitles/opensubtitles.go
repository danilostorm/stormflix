package subtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OpenSubtitlesProvider struct {
	apiKey    string
	username  string
	password  string
	userAgent string
	client    *http.Client
	mu        sync.Mutex
	token     string
}

func NewOpenSubtitlesProvider(apiKey, username, password, userAgent string) *OpenSubtitlesProvider {
	return &OpenSubtitlesProvider{
		apiKey:    strings.TrimSpace(apiKey),
		username:  strings.TrimSpace(username),
		password:  password,
		userAgent: strings.TrimSpace(userAgent),
		client:    &http.Client{Timeout: 25 * time.Second},
	}
}

func (p *OpenSubtitlesProvider) Name() string { return "opensubtitles" }
func (p *OpenSubtitlesProvider) Ready() bool {
	return p.apiKey != "" && p.username != "" && p.password != ""
}

func (p *OpenSubtitlesProvider) Download(ctx context.Context, query Query) (Download, error) {
	if !p.Ready() {
		return Download{}, errors.New("OpenSubtitles is not configured")
	}
	token, err := p.login(ctx)
	if err != nil {
		return Download{}, err
	}
	q := url.Values{}
	if query.TMDBID > 0 {
		q.Set("tmdb_id", strconv.FormatInt(query.TMDBID, 10))
	} else if query.IMDbID != "" {
		q.Set("imdb_id", strings.TrimPrefix(query.IMDbID, "tt"))
	} else {
		return Download{}, errors.New("OpenSubtitles: metadata ID missing")
	}
	q.Set("languages", normalizeOpenSubtitlesLanguage(query.Language))
	if query.Season > 0 {
		q.Set("season_number", strconv.Itoa(query.Season))
	}
	if query.Episode > 0 {
		q.Set("episode_number", strconv.Itoa(query.Episode))
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.opensubtitles.com/api/v1/subtitles?"+q.Encode(), nil)
	p.headers(req, token)
	resp, err := p.client.Do(req)
	if err != nil {
		return Download{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("OpenSubtitles search: HTTP %d", resp.StatusCode)
	}
	var search struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Language        string `json:"language"`
				Release         string `json:"release"`
				HearingImpaired bool   `json:"hearing_impaired"`
				Files           []struct {
					FileID   int64  `json:"file_id"`
					FileName string `json:"file_name"`
				} `json:"files"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		return Download{}, err
	}
	if len(search.Data) == 0 || len(search.Data[0].Attributes.Files) == 0 {
		return Download{}, errors.New("OpenSubtitles: no subtitle found")
	}
	candidate := search.Data[0]
	file := candidate.Attributes.Files[0]
	requestBody, _ := json.Marshal(map[string]any{"file_id": file.FileID, "sub_format": "srt"})
	downloadReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.opensubtitles.com/api/v1/download", bytes.NewReader(requestBody))
	p.headers(downloadReq, token)
	downloadReq.Header.Set("Content-Type", "application/json")
	downloadResp, err := p.client.Do(downloadReq)
	if err != nil {
		return Download{}, err
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode < 200 || downloadResp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("OpenSubtitles download request: HTTP %d", downloadResp.StatusCode)
	}
	var ticket struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(downloadResp.Body).Decode(&ticket); err != nil {
		return Download{}, err
	}
	if ticket.Link == "" {
		return Download{}, errors.New("OpenSubtitles: download link missing")
	}
	fileReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, ticket.Link, nil)
	fileReq.Header.Set("User-Agent", p.userAgent)
	fileResp, err := p.client.Do(fileReq)
	if err != nil {
		return Download{}, err
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode < 200 || fileResp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("OpenSubtitles subtitle file: HTTP %d", fileResp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(fileResp.Body, 8<<20))
	if err != nil {
		return Download{}, err
	}
	return Download{
		Provider:         "opensubtitles",
		ProviderID:       candidate.ID + ":" + strconv.FormatInt(file.FileID, 10),
		Language:         candidate.Attributes.Language,
		ReleaseName:      firstSubtitleName(candidate.Attributes.Release, file.FileName),
		Format:           "srt",
		SourceURL:        ticket.Link,
		HearingImpaired: candidate.Attributes.HearingImpaired,
		Data:             data,
	}, nil
}

func (p *OpenSubtitlesProvider) login(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" {
		return p.token, nil
	}
	body, _ := json.Marshal(map[string]string{"username": p.username, "password": p.password})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.opensubtitles.com/api/v1/login", bytes.NewReader(body))
	p.headers(req, "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenSubtitles login: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if data.Token == "" {
		return "", errors.New("OpenSubtitles login token missing")
	}
	p.token = data.Token
	return p.token, nil
}

func (p *OpenSubtitlesProvider) headers(req *http.Request, token string) {
	req.Header.Set("Api-Key", p.apiKey)
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func normalizeOpenSubtitlesLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	switch language {
	case "pt-br", "pt_br", "pob":
		return "pt-BR"
	case "pt-pt", "por":
		return "pt"
	default:
		if len(language) >= 2 {
			return language
		}
		return "pt-BR"
	}
}

func firstSubtitleName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "subtitle"
}
