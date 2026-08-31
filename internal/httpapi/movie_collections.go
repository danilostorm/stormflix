package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
)

var movieCollectionIndexers sync.Map

// startMovieCollectionIndexer lazily starts when an authenticated Home is used.
// It backfills existing matched movies and then wakes periodically for newly
// matched titles. Work is intentionally serial and rate-limited so collection
// discovery never competes aggressively with playback or remote scans.
func (s *server) startMovieCollectionIndexer() {
	if _, loaded := movieCollectionIndexers.LoadOrStore(s, struct{}{}); loaded {
		return
	}
	go func() {
		for {
			if !s.metadata.TMDBReady() {
				time.Sleep(5 * time.Minute)
				continue
			}
			worked, err := s.indexOneMovieCollection(context.Background())
			switch {
			case err != nil:
				time.Sleep(30 * time.Second)
			case worked:
				time.Sleep(350 * time.Millisecond)
			default:
				time.Sleep(5 * time.Minute)
			}
		}
	}()
}

func (s *server) indexOneMovieCollection(ctx context.Context) (bool, error) {
	var mediaID, tmdbID int64
	err := s.db.QueryRowContext(ctx, `
SELECT mm.media_id,mm.tmdb_id
FROM media_metadata mm
JOIN media m ON m.id=mm.media_id
JOIN libraries l ON l.id=m.library_id
WHERE m.available=1 AND l.kind<>'music'
  AND mm.media_type='movie' AND mm.tmdb_id>0
  AND (COALESCE(mm.collection_checked_at,'')='' OR COALESCE(mm.collection_source_tmdb_id,0)<>mm.tmdb_id)
ORDER BY mm.media_id
LIMIT 1`).Scan(&mediaID, &tmdbID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	collection, err := s.metadata.MovieCollection(ctx, tmdbID)
	if err != nil {
		return true, err
	}
	name := strings.TrimSpace(collection.Name)
	_, err = s.db.ExecContext(ctx, `
UPDATE media_metadata
SET collection_tmdb_id=?,collection_name=?,collection_source_tmdb_id=?,collection_checked_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
WHERE media_id=? AND tmdb_id=?`, collection.ID, name, tmdbID, mediaID, tmdbID)
	if err != nil {
		return true, err
	}
	return true, nil
}
