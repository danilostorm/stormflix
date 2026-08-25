package subtitles

import "context"

type Query struct {
	MediaID   int64
	TMDBID    int64
	IMDbID    string
	MediaType string
	Season    int
	Episode   int
	Language  string
}

type Download struct {
	Provider     string
	ProviderID   string
	Language     string
	ReleaseName  string
	Format       string
	SourceURL    string
	HearingImpaired bool
	Data         []byte
}

type Provider interface {
	Name() string
	Ready() bool
	Download(ctx context.Context, query Query) (Download, error)
}

type AgentStatus struct {
	Name        string `json:"name"`
	Ready       bool   `json:"ready"`
	Description string `json:"description"`
}
