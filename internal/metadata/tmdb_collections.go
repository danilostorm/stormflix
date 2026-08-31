package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

// MovieCollectionRef is TMDB's belongs_to_collection identity. StormFlix uses
// the stable TMDB collection id rather than title heuristics so unrelated films
// are never grouped merely because their names look similar.
type MovieCollectionRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (p *TMDBProvider) MovieCollection(ctx context.Context, movieID int64) (MovieCollectionRef, error) {
	if !p.Ready() {
		return MovieCollectionRef{}, errors.New("TMDB is not configured")
	}
	if movieID <= 0 {
		return MovieCollectionRef{}, errors.New("invalid TMDB movie id")
	}
	q := url.Values{}
	if p.language != "" {
		q.Set("language", p.language)
	}
	var response struct {
		BelongsToCollection *MovieCollectionRef `json:"belongs_to_collection"`
	}
	rawURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d", movieID)
	if encoded := q.Encode(); encoded != "" {
		rawURL += "?" + encoded
	}
	if err := p.get(ctx, rawURL, &response); err != nil {
		return MovieCollectionRef{}, err
	}
	if response.BelongsToCollection == nil || response.BelongsToCollection.ID <= 0 {
		return MovieCollectionRef{}, nil
	}
	return *response.BelongsToCollection, nil
}

// MovieCollection keeps provider access behind the metadata service lock so a
// live Admin credentials update cannot race the background collection indexer.
func (s *Service) MovieCollection(ctx context.Context, movieID int64) (MovieCollectionRef, error) {
	s.providerMu.RLock()
	tmdb := s.tmdb
	s.providerMu.RUnlock()
	if tmdb == nil || !tmdb.Ready() {
		return MovieCollectionRef{}, errors.New("TMDB is not configured")
	}
	return tmdb.MovieCollection(ctx, movieID)
}

func (s *Service) TMDBReady() bool {
	s.providerMu.RLock()
	defer s.providerMu.RUnlock()
	return s.tmdb != nil && s.tmdb.Ready()
}
