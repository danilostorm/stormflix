package metadata

import "context"

type SourceItem struct {
	ID          int64
	LibraryID   int64
	LibraryKind string
	Title       string
	Path        string
	SourceRoot  string
	SeriesKey   string
	SeriesTitle string
	Season      int
	Episode     int
	Absolute    int
}

// Parsed returns filename metadata enriched by the scanner-owned series
// identity. Providers never have to infer the show from release noise when the
// library structure already told StormFlix which series/season/episode it is.
func (s SourceItem) Parsed() ParsedName {
	parsed := ParseFilename(s.Path, s.LibraryKind)
	if s.SeriesTitle != "" {
		if parsed.Title != "" && normalizeTitle(parsed.Title) != normalizeTitle(s.SeriesTitle) {
			parsed.Alternates = append([]string{parsed.Title}, parsed.Alternates...)
		}
		parsed.Title = s.SeriesTitle
	}
	if s.Season > 0 {
		parsed.Season = s.Season
	}
	if s.Episode > 0 {
		parsed.Episode = s.Episode
	}
	if parsed.Season == 0 && parsed.Episode > 0 && isSeriesLibraryKind(s.LibraryKind) {
		parsed.Season = 1
	}
	parsed.Alternates = uniqueTitles(parsed.Title, parsed.Alternates)
	return parsed
}

type Artwork struct {
	Kind     string
	Provider string
	URL      string
	Language string
	Score    float64
}

type Person struct {
	Name       string `json:"name"`
	Character  string `json:"character,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
}

type Result struct {
	Title             string
	OriginalTitle     string
	Tagline           string
	MediaType         string
	Year              int
	ReleaseDate       string
	ContentRating     string
	ContentRatingAge  int
	Season            int
	Episode           int
	Overview          string
	Genres            []string
	Rating            float64
	RuntimeMinutes    int
	Provider          string
	ProviderID        string
	TMDBID            int64
	TVDBID            int64
	IMDbID            string
	AniListID         int64
	MALID             int64
	Cast              []Person
	Directors         []string
	TrailerURL        string
	ThemePreviewURL   string
	ThemePreviewTitle string
	Artwork           []Artwork
}

type Provider interface {
	Name() string
	Ready() bool
	Supports(kind string) bool
	Lookup(ctx context.Context, item SourceItem, parsed ParsedName) (Result, error)
}

type AgentStatus struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Ready       bool   `json:"ready"`
	Description string `json:"description"`
}
