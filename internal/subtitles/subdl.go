package subtitles

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SubDLProvider struct {
	apiKey string
	client *http.Client
}

func NewSubDLProvider(apiKey string) *SubDLProvider {
	return &SubDLProvider{apiKey: strings.TrimSpace(apiKey), client: &http.Client{Timeout: 25 * time.Second}}
}

func (p *SubDLProvider) Name() string { return "subdl" }
func (p *SubDLProvider) Ready() bool { return p.apiKey != "" }

func (p *SubDLProvider) Download(ctx context.Context, query Query) (Download, error) {
	if !p.Ready() {
		return Download{}, errors.New("SubDL is not configured")
	}
	q := url.Values{}
	q.Set("api_key", p.apiKey)
	q.Set("unpack", "1")
	q.Set("releases", "1")
	q.Set("languages", normalizeSubDLLanguage(query.Language))
	q.Set("client", "custom_integration")
	if query.TMDBID > 0 {
		q.Set("tmdb_id", strconv.FormatInt(query.TMDBID, 10))
	} else if query.IMDbID != "" {
		q.Set("imdb_id", strings.TrimPrefix(query.IMDbID, "tt"))
	} else {
		return Download{}, errors.New("SubDL: metadata ID missing")
	}
	if query.MediaType == "movie" {
		q.Set("type", "movie")
	} else {
		q.Set("type", "tv")
		if query.Season > 0 {
			q.Set("season_number", strconv.Itoa(query.Season))
		}
		if query.Episode > 0 {
			q.Set("episode_number", strconv.Itoa(query.Episode))
		}
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.subdl.com/api/v1/subtitles?"+q.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.3")
	resp, err := p.client.Do(req)
	if err != nil {
		return Download{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Download{}, fmt.Errorf("SubDL search: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Status    bool   `json:"status"`
		Error     string `json:"error"`
		Subtitles []struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			Language    string `json:"language"`
			ReleaseName string `json:"release_name"`
			HI          bool   `json:"hi"`
			Format      string `json:"format"`
			Season      int    `json:"season"`
			Episode     int    `json:"episode"`
			UnpackFiles []struct {
				FileNID     string `json:"file_n_id"`
				Name        string `json:"name"`
				ReleaseName string `json:"release_name"`
				Season      int    `json:"season"`
				Episode     int    `json:"episode"`
				Language    string `json:"language"`
				HI          bool   `json:"hi"`
				Format      string `json:"format"`
				URL         string `json:"url"`
			} `json:"unpack_files"`
		} `json:"subtitles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Download{}, err
	}
	if !data.Status && data.Error != "" {
		return Download{}, errors.New("SubDL: " + data.Error)
	}
	if len(data.Subtitles) == 0 {
		return Download{}, errors.New("SubDL: no subtitle found")
	}

	for _, subtitle := range data.Subtitles {
		for _, file := range subtitle.UnpackFiles {
			if query.Season > 0 && file.Season > 0 && file.Season != query.Season {
				continue
			}
			if query.Episode > 0 && file.Episode > 0 && file.Episode != query.Episode {
				continue
			}
			if file.URL == "" {
				continue
			}
			rawURL := absoluteSubDLURL(file.URL)
			body, err := p.download(ctx, rawURL)
			if err != nil {
				continue
			}
			format := normalizeSubtitleFormat(firstSubtitleName(file.Format, filepath.Ext(file.Name)))
			return Download{Provider: "subdl", ProviderID: file.FileNID, Language: firstSubtitleName(file.Language, query.Language), ReleaseName: firstSubtitleName(file.ReleaseName, file.Name), Format: format, SourceURL: rawURL, HearingImpaired: file.HI, Data: body}, nil
		}
	}

	for i, subtitle := range data.Subtitles {
		if subtitle.URL == "" {
			continue
		}
		rawURL := absoluteSubDLURL(subtitle.URL)
		body, err := p.download(ctx, rawURL)
		if err != nil {
			continue
		}
		format := normalizeSubtitleFormat(firstSubtitleName(subtitle.Format, filepath.Ext(subtitle.Name)))
		if strings.HasSuffix(strings.ToLower(rawURL), ".zip") || bytes.HasPrefix(body, []byte("PK")) {
			unzipped, ext, unzipErr := firstSubtitleFromZIP(body)
			if unzipErr != nil {
				continue
			}
			body = unzipped
			format = ext
		}
		return Download{Provider: "subdl", ProviderID: strconv.Itoa(i + 1) + ":" + subtitle.URL, Language: firstSubtitleName(subtitle.Language, query.Language), ReleaseName: firstSubtitleName(subtitle.ReleaseName, subtitle.Name), Format: format, SourceURL: rawURL, HearingImpaired: subtitle.HI, Data: body}, nil
	}
	return Download{}, errors.New("SubDL: subtitles were found but could not be downloaded")
}

func (p *SubDLProvider) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	req.Header.Set("User-Agent", "StormFlix/0.3")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SubDL download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

func firstSubtitleFromZIP(data []byte) ([]byte, string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", err
	}
	for _, file := range reader.File {
		ext := normalizeSubtitleFormat(filepath.Ext(file.Name))
		if ext != "srt" && ext != "ass" && ext != "vtt" && ext != "sub" {
			continue
		}
		r, err := file.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(r, 8<<20))
		_ = r.Close()
		if err == nil && len(body) > 0 {
			return body, ext, nil
		}
	}
	return nil, "", errors.New("SubDL zip did not contain a supported subtitle")
}

func absoluteSubDLURL(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return "https://dl.subdl.com/subtitle" + strings.TrimPrefix(value, "/subtitle")
}

func normalizeSubDLLanguage(language string) string {
	language = strings.ToUpper(strings.TrimSpace(language))
	switch language {
	case "PT-BR", "PT_BR", "POB":
		return "PT"
	case "PT-PT", "POR":
		return "PT"
	default:
		if len(language) >= 2 {
			return language[:2]
		}
		return "PT"
	}
}

func normalizeSubtitleFormat(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, ".")
	switch value {
	case "srt", "ass", "vtt", "sub":
		return value
	default:
		return "srt"
	}
}
