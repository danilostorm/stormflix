package metadata

import "context"

type SourceItem struct {
	ID          int64
	LibraryID   int64
	LibraryKind string
	Title       string
	Path        string
}

type Artwork struct {
	Kind     string
	Provider string
	URL      string
	Language string
	Score    float64
}

type Result struct {
	Title          string
	MediaType      string
	Year           int
	Season         int
	Episode        int
	Overview       string
	Genres         []string
	Rating         float64
	RuntimeMinutes int
	Provider       string
	ProviderID     string
	TMDBID         int64
	TVDBID         int64
	IMDbID         string
	AniListID      int64
	MALID          int64
	Artwork        []Artwork
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
