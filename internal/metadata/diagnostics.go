package metadata

import (
	"context"
	"errors"
)

func (s *Service) TestTMDB(ctx context.Context) error {
	if s.tmdb == nil || !s.tmdb.Ready() {
		return errors.New("TMDB is not configured")
	}
	var response struct {
		Images struct {
			BaseURL string `json:"base_url"`
		} `json:"images"`
	}
	return s.tmdb.get(ctx, "https://api.themoviedb.org/3/configuration", &response)
}
