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
	"time"
)

type TMDBProvider struct {
	token    string
	apiKey   string
	language string
	client   *http.Client
}

func NewTMDBProvider(token, apiKey, language string) *TMDBProvider {
	return &TMDBProvider{
		token:    strings.TrimSpace(token),
		apiKey:   strings.TrimSpace(apiKey),
		language: strings.TrimSpace(language),
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *TMDBProvider) Name() string { return "tmdb" }
func (p *TMDBProvider) Ready() bool { return p.token != "" || p.apiKey != "" }
func (p *TMDBProvider) Supports(kind string) bool {
	return kind == "movies" || kind == "series" || kind == "anime" || kind == "mixed" || kind == "anime_series"
}

func (p *TMDBProvider) Lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error) {
	if !p.Ready() {
		return Result{}, errors.New("TMDB is not configured")
	}

	titles := parsed.SearchTitles()
	if len(titles) == 0 {
		titles = []string{parsed.Title}
	}
	animeResolved := item.LibraryKind == "anime" || item.LibraryKind == "anime_series"
	if item.LibraryKind == "anime" || item.LibraryKind == "mixed" || item.LibraryKind == "anime_series" {
		if match, err := defaultAniDBResolver.Resolve(ctx, titles); err == nil && strings.TrimSpace(match.Title) != "" {
			animeResolved = true
			titles = prependSearchTitle(match.Title, titles)
		}
	}

	mediaTypes := []string{"tv"}
	switch item.LibraryKind {
	case "movies":
		mediaTypes = []string{"movie"}
	case "series", "anime_series":
		// Dubbed anime with seasons behaves like a television series for TMDB
		// matching. Anime providers are still available in the fallback pass.
		mediaTypes = []string{"tv"}
	case "anime":
		if parsed.LikelyMovie {
			mediaTypes = []string{"movie", "tv"}
		} else {
			mediaTypes = []string{"tv", "movie"}
		}
	case "mixed":
		mediaTypes = []string{"movie", "tv"}
	default:
		mediaTypes = []string{"movie", "tv"}
	}

	searched := make([]string, 0, len(mediaTypes)*len(titles))
	for _, mediaType := range mediaTypes {
		for _, title := range titles {
			searched = append(searched, mediaType+":"+title)
			id, err := p.search(ctx, mediaType, title, parsed.Year)
			if err != nil {
				return Result{}, err
			}
			if id == 0 {
				continue
			}
			var result Result
			if mediaType == "movie" {
				result, err = p.movie(ctx, id, parsed)
			} else {
				result, err = p.tv(ctx, id, parsed)
			}
			if err != nil {
				return Result{}, err
			}
			if animeResolved {
				result.MediaType = "anime"
			}
			return result, nil
		}
	}
	return Result{}, fmt.Errorf("TMDB: no match; tried %s", strings.Join(searched, " | "))
}

func prependSearchTitle(value string, items []string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	key := normalizeTitle(value)
	out := []string{value}
	for _, item := range items {
		if normalizeTitle(item) != key {
			out = append(out, item)
		}
	}
	return out
}

type tmdbSearchResponse struct {
	Results []struct {
		ID            int64   `json:"id"`
		Title         string  `json:"title"`
		OriginalTitle string  `json:"original_title"`
		Name          string  `json:"name"`
		OriginalName  string  `json:"original_name"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		Popularity    float64 `json:"popularity"`
	} `json:"results"`
}

func (p *TMDBProvider) search(ctx context.Context, mediaType, title string, year int) (int64, error) {
	endpoint := "https://api.themoviedb.org/3/search/" + mediaType
	q := url.Values{}
	q.Set("query", title)
	q.Set("include_adult", "false")
	if p.language != "" {
		q.Set("language", p.language)
	}
	if year > 0 {
		if mediaType == "movie" {
			q.Set("year", strconv.Itoa(year))
		} else {
			q.Set("first_air_date_year", strconv.Itoa(year))
		}
	}
	var response tmdbSearchResponse
	if err := p.get(ctx, endpoint+"?"+q.Encode(), &response); err != nil {
		return 0, err
	}
	if len(response.Results) == 0 && year > 0 {
		q.Del("year")
		q.Del("first_air_date_year")
		if err := p.get(ctx, endpoint+"?"+q.Encode(), &response); err != nil {
			return 0, err
		}
	}
	if len(response.Results) == 0 {
		return 0, nil
	}

	want := normalizeTitle(title)
	bestID := response.Results[0].ID
	bestScore := -1.0
	for _, candidate := range response.Results {
		names := []string{candidate.Title, candidate.OriginalTitle, candidate.Name, candidate.OriginalName}
		score := candidate.Popularity / 100000
		for _, name := range names {
			n := normalizeTitle(name)
			if n == want && n != "" {
				score += 10
			} else if strings.Contains(n, want) || strings.Contains(want, n) {
				score += 2
			}
		}
		candidateYear := yearFromDate(candidate.ReleaseDate)
		if candidateYear == 0 {
			candidateYear = yearFromDate(candidate.FirstAirDate)
		}
		if year > 0 && candidateYear == year {
			score += 4
		}
		if score > bestScore {
			bestScore = score
			bestID = candidate.ID
		}
	}
	return bestID, nil
}

type tmdbGenre struct {
	Name string `json:"name"`
}

type tmdbImage struct {
	FilePath    string  `json:"file_path"`
	ISO6391     string  `json:"iso_639_1"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

type tmdbImages struct {
	Posters   []tmdbImage `json:"posters"`
	Backdrops []tmdbImage `json:"backdrops"`
	Logos     []tmdbImage `json:"logos"`
}

type tmdbExternalIDs struct {
	IMDbID string `json:"imdb_id"`
	TVDBID int64  `json:"tvdb_id"`
}

type tmdbMovie struct {
	ID           int64           `json:"id"`
	Title        string          `json:"title"`
	Overview     string          `json:"overview"`
	ReleaseDate  string          `json:"release_date"`
	VoteAverage  float64         `json:"vote_average"`
	Runtime      int             `json:"runtime"`
	IMDbID       string          `json:"imdb_id"`
	Genres       []tmdbGenre     `json:"genres"`
	PosterPath   string          `json:"poster_path"`
	BackdropPath string          `json:"backdrop_path"`
	Images       tmdbImages      `json:"images"`
	ExternalIDs  tmdbExternalIDs `json:"external_ids"`
}

func (p *TMDBProvider) movie(ctx context.Context, id int64, parsed ParsedName) (Result, error) {
	q := url.Values{}
	if p.language != "" {
		q.Set("language", p.language)
	}
	q.Set("append_to_response", "images,external_ids")
	q.Set("include_image_language", imageLanguages(p.language))
	var movie tmdbMovie
	if err := p.get(ctx, fmt.Sprintf("https://api.themoviedb.org/3/movie/%d?%s", id, q.Encode()), &movie); err != nil {
		return Result{}, err
	}
	result := Result{
		Title:          movie.Title,
		MediaType:      "movie",
		Year:           yearFromDate(movie.ReleaseDate),
		Overview:       movie.Overview,
		Genres:         genreNames(movie.Genres),
		Rating:         movie.VoteAverage,
		RuntimeMinutes: movie.Runtime,
		Provider:       "tmdb",
		ProviderID:     strconv.FormatInt(movie.ID, 10),
		TMDBID:         movie.ID,
		IMDbID:         firstNonEmpty(movie.IMDbID, movie.ExternalIDs.IMDbID),
	}
	result.Artwork = appendTMDBArtwork(result.Artwork, "poster", movie.PosterPath, "w500", "", 100)
	result.Artwork = appendTMDBArtwork(result.Artwork, "backdrop", movie.BackdropPath, "original", "", 100)
	if logo := bestImage(movie.Images.Logos, p.language); logo.FilePath != "" {
		result.Artwork = appendTMDBArtwork(result.Artwork, "logo", logo.FilePath, "original", logo.ISO6391, 110+logo.VoteAverage)
	}
	return result, nil
}

type tmdbTV struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Overview       string          `json:"overview"`
	FirstAirDate   string          `json:"first_air_date"`
	VoteAverage    float64         `json:"vote_average"`
	EpisodeRunTime []int           `json:"episode_run_time"`
	Genres         []tmdbGenre     `json:"genres"`
	PosterPath     string          `json:"poster_path"`
	BackdropPath   string          `json:"backdrop_path"`
	Images         tmdbImages      `json:"images"`
	ExternalIDs    tmdbExternalIDs `json:"external_ids"`
}

type tmdbEpisode struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Overview    string  `json:"overview"`
	StillPath   string  `json:"still_path"`
	Runtime     int     `json:"runtime"`
	VoteAverage float64 `json:"vote_average"`
	AirDate     string  `json:"air_date"`
}

func (p *TMDBProvider) tv(ctx context.Context, id int64, parsed ParsedName) (Result, error) {
	q := url.Values{}
	if p.language != "" {
		q.Set("language", p.language)
	}
	q.Set("append_to_response", "images,external_ids")
	q.Set("include_image_language", imageLanguages(p.language))
	var show tmdbTV
	if err := p.get(ctx, fmt.Sprintf("https://api.themoviedb.org/3/tv/%d?%s", id, q.Encode()), &show); err != nil {
		return Result{}, err
	}
	runtimeMinutes := 0
	if len(show.EpisodeRunTime) > 0 {
		runtimeMinutes = show.EpisodeRunTime[0]
	}
	result := Result{
		Title:          show.Name,
		MediaType:      "series",
		Year:           yearFromDate(show.FirstAirDate),
		Season:         parsed.Season,
		Episode:        parsed.Episode,
		Overview:       show.Overview,
		Genres:         genreNames(show.Genres),
		Rating:         show.VoteAverage,
		RuntimeMinutes: runtimeMinutes,
		Provider:       "tmdb",
		ProviderID:     strconv.FormatInt(show.ID, 10),
		TMDBID:         show.ID,
		TVDBID:         show.ExternalIDs.TVDBID,
		IMDbID:         show.ExternalIDs.IMDbID,
	}
	result.Artwork = appendTMDBArtwork(result.Artwork, "poster", show.PosterPath, "w500", "", 100)
	result.Artwork = appendTMDBArtwork(result.Artwork, "backdrop", show.BackdropPath, "original", "", 100)
	if logo := bestImage(show.Images.Logos, p.language); logo.FilePath != "" {
		result.Artwork = appendTMDBArtwork(result.Artwork, "logo", logo.FilePath, "original", logo.ISO6391, 110+logo.VoteAverage)
	}

	if parsed.Season > 0 && parsed.Episode > 0 {
		var episode tmdbEpisode
		epURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%d/episode/%d", id, parsed.Season, parsed.Episode)
		epQ := url.Values{}
		if p.language != "" {
			epQ.Set("language", p.language)
		}
		if err := p.get(ctx, epURL+"?"+epQ.Encode(), &episode); err == nil && episode.ID != 0 {
			if episode.Name != "" {
				result.Title = fmt.Sprintf("%s S%02dE%02d · %s", show.Name, parsed.Season, parsed.Episode, episode.Name)
			}
			if episode.Overview != "" {
				result.Overview = episode.Overview
			}
			if episode.Runtime > 0 {
				result.RuntimeMinutes = episode.Runtime
			}
			if episode.VoteAverage > 0 {
				result.Rating = episode.VoteAverage
			}
			if episode.StillPath != "" {
				result.Artwork = appendTMDBArtwork(result.Artwork, "backdrop", episode.StillPath, "original", "", 120)
			}
		}
	}
	return result, nil
}

func (p *TMDBProvider) get(ctx context.Context, rawURL string, dest any) error {
	if p.apiKey != "" && p.token == "" {
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		q := u.Query()
		q.Set("api_key", p.apiKey)
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix/0.6")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("TMDB: not found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TMDB: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func appendTMDBArtwork(items []Artwork, kind, filePath, size, language string, score float64) []Artwork {
	if filePath == "" {
		return items
	}
	return append(items, Artwork{Kind: kind, Provider: "tmdb", URL: "https://image.tmdb.org/t/p/" + size + filePath, Language: language, Score: score})
}

func bestImage(images []tmdbImage, language string) tmdbImage {
	lang := strings.ToLower(strings.Split(language, "-")[0])
	best := tmdbImage{}
	bestScore := -1.0
	for _, image := range images {
		score := image.VoteAverage + float64(image.VoteCount)/100
		imageLang := strings.ToLower(image.ISO6391)
		if imageLang == lang {
			score += 20
		} else if imageLang == "en" {
			score += 10
		} else if imageLang == "" {
			score += 5
		}
		if score > bestScore {
			best = image
			bestScore = score
		}
	}
	return best
}

func imageLanguages(language string) string {
	lang := strings.ToLower(strings.Split(language, "-")[0])
	if lang == "" || lang == "en" {
		return "en,null"
	}
	return lang + ",en,null"
}

func normalizeTitle(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 128 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func yearFromDate(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, _ := strconv.Atoi(value[:4])
	return year
}

func genreNames(genres []tmdbGenre) []string {
	out := make([]string, 0, len(genres))
	for _, genre := range genres {
		if genre.Name != "" {
			out = append(out, genre.Name)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
