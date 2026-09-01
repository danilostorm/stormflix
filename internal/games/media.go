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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GameMedia contains presentation-only metadata for the public game detail
// page. Provider credentials never leave the server.
type GameMedia struct {
	Screenshots     []string `json:"screenshots"`
	TrailerURL      string   `json:"trailer_url"`
	Genres          []string `json:"genres"`
	Developers      []string `json:"developers"`
	Publishers      []string `json:"publishers"`
	CommunityRating float64  `json:"community_rating"`
}

type gameTrailerCacheEntry struct {
	URL       string
	ExpiresAt time.Time
}

var (
	gameTrailerCache sync.Map
	youtubeVideoID   = regexp.MustCompile(`^[A-Za-z0-9_-]{6,24}$`)
)

func decodeStringList(raw string) []string {
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func publicImageURLs(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		if len(out) >= limit {
			break
		}
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		out = append(out, parsed.String())
	}
	return out
}

// Media reads metadata already collected by the Games metadata worker and, when
// IGDB is configured, resolves the provider's video reference to a YouTube
// trailer URL. Trailer lookups are cached so opening a detail page never turns
// into a provider polling loop.
func (s *Service) Media(ctx context.Context, gameID int64) (GameMedia, error) {
	if gameID <= 0 {
		return GameMedia{}, errors.New("invalid game id")
	}
	var screenshots, genres, developers, publishers, igdbID string
	var rating float64
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(gm.screenshots_json,'[]'),COALESCE(gm.genres_json,'[]'),
       COALESCE(gm.developers_json,'[]'),COALESCE(gm.publishers_json,'[]'),
       COALESCE(gm.community_rating,0),COALESCE(gm.igdb_id,'')
FROM games g
LEFT JOIN game_metadata gm ON gm.game_id=g.id
WHERE g.id=?`, gameID).Scan(&screenshots, &genres, &developers, &publishers, &rating, &igdbID)
	if err != nil {
		return GameMedia{}, err
	}
	media := GameMedia{
		Screenshots:     publicImageURLs(decodeStringList(screenshots), 8),
		Genres:          decodeStringList(genres),
		Developers:      decodeStringList(developers),
		Publishers:      decodeStringList(publishers),
		CommunityRating: rating,
	}
	igdbID = strings.TrimSpace(igdbID)
	if igdbID == "" {
		return media, nil
	}
	if cached, ok := gameTrailerCache.Load(igdbID); ok {
		entry := cached.(gameTrailerCacheEntry)
		if time.Now().Before(entry.ExpiresAt) {
			media.TrailerURL = entry.URL
			return media, nil
		}
		gameTrailerCache.Delete(igdbID)
	}
	public, secrets, enabled, err := s.ProviderSecretsForRuntime(ctx, "igdb")
	if err != nil || !enabled || strings.TrimSpace(public["client_id"]) == "" || strings.TrimSpace(secrets["client_secret"]) == "" {
		return media, nil
	}
	trailer, err := fetchIGDBTrailer(ctx, public["client_id"], secrets["client_secret"], igdbID)
	if err == nil {
		media.TrailerURL = trailer
		gameTrailerCache.Store(igdbID, gameTrailerCacheEntry{URL: trailer, ExpiresAt: time.Now().Add(6 * time.Hour)})
	}
	return media, nil
}

func fetchIGDBTrailer(ctx context.Context, clientID, clientSecret, igdbID string) (string, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(igdbID), 10, 64)
	if err != nil || id <= 0 {
		return "", errors.New("invalid IGDB id")
	}
	client := &http.Client{Timeout: 12 * time.Second}
	tokenURL := "https://id.twitch.tv/oauth2/token?client_id=" + url.QueryEscape(clientID) + "&client_secret=" + url.QueryEscape(clientSecret) + "&grant_type=client_credentials"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("IGDB OAuth HTTP %d", resp.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil || strings.TrimSpace(token.AccessToken) == "" {
		return "", errors.New("invalid IGDB OAuth token")
	}
	query := fmt.Sprintf("fields videos.name,videos.video_id; where id = (%d); limit 1;", id)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.igdb.com/v4/games", bytes.NewBufferString(query))
	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "text/plain")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("IGDB video HTTP %d", resp.StatusCode)
	}
	var result []struct {
		Videos []struct {
			Name    string `json:"name"`
			VideoID string `json:"video_id"`
		} `json:"videos"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || len(result) == 0 {
		return "", err
	}
	best := ""
	for _, video := range result[0].Videos {
		videoID := strings.TrimSpace(video.VideoID)
		if !youtubeVideoID.MatchString(videoID) {
			continue
		}
		if best == "" {
			best = videoID
		}
		name := strings.ToLower(strings.TrimSpace(video.Name))
		if strings.Contains(name, "trailer") || strings.Contains(name, "launch") || strings.Contains(name, "teaser") {
			best = videoID
			break
		}
	}
	if best == "" {
		return "", nil
	}
	return "https://www.youtube.com/watch?v=" + url.QueryEscape(best), nil
}
