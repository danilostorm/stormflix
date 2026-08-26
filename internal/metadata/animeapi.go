package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AnimeAPIProvider struct {
	client *http.Client
	base   string
}

type animeAPIMapping struct {
	Title            string `json:"title"`
	AniDB            int64  `json:"anidb"`
	AniList          int64  `json:"anilist"`
	IMDb             string `json:"imdb"`
	MyAnimeList      int64  `json:"myanimelist"`
	TheMovieDB       int64  `json:"themoviedb"`
	TheMovieDBType   string `json:"themoviedb_type"`
	TheTVDB          int64  `json:"thetvdb"`
}

func NewAnimeAPIProvider() *AnimeAPIProvider {
	return &AnimeAPIProvider{client: &http.Client{Timeout: 8 * time.Second}, base: "https://animeapi.my.id"}
}

func (p *AnimeAPIProvider) Ready() bool { return p != nil && p.client != nil }

// Enrich maps IDs already discovered by AniList/MAL/TMDB to the equivalent IDs
// used by other anime databases. The relation service is deliberately optional:
// failures never make metadata matching fail.
func (p *AnimeAPIProvider) Enrich(ctx context.Context, result *Result) error {
	if !p.Ready() || result == nil {
		return nil
	}
	platform, id := "", ""
	switch {
	case result.MALID > 0:
		platform, id = "myanimelist", strconv.FormatInt(result.MALID, 10)
	case result.AniListID > 0:
		platform, id = "anilist", strconv.FormatInt(result.AniListID, 10)
	case result.TMDBID > 0:
		kind := "tv"
		if result.MediaType == "movie" {
			kind = "movie"
		}
		platform, id = "themoviedb", kind+"/"+strconv.FormatInt(result.TMDBID, 10)
	default:
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%s", p.base, platform, id), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "StormFlix/0.7")
	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("AnimeAPI HTTP %d", response.StatusCode)
	}
	var mapping animeAPIMapping
	if err := json.NewDecoder(response.Body).Decode(&mapping); err != nil {
		return err
	}
	if result.AniListID == 0 {
		result.AniListID = mapping.AniList
	}
	if result.MALID == 0 {
		result.MALID = mapping.MyAnimeList
	}
	if result.TMDBID == 0 {
		result.TMDBID = mapping.TheMovieDB
	}
	if result.TVDBID == 0 {
		result.TVDBID = mapping.TheTVDB
	}
	if strings.TrimSpace(result.IMDbID) == "" {
		result.IMDbID = strings.TrimSpace(mapping.IMDb)
	}
	return nil
}
