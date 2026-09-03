package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Item struct {
	ID               int64    `json:"id"`
	LibraryID        int64    `json:"library_id"`
	LibraryName      string   `json:"library_name"`
	Title            string   `json:"title"`
	Extension        string   `json:"extension"`
	SizeBytes        int64    `json:"size_bytes"`
	ModifiedUnix     int64    `json:"modified_unix"`
	Available        bool     `json:"available"`
	MediaType        string   `json:"media_type"`
	Year             int      `json:"year"`
	SeasonNumber     int      `json:"season_number"`
	EpisodeNumber    int      `json:"episode_number"`
	Overview         string   `json:"overview"`
	Genres           []string `json:"genres"`
	Rating           float64  `json:"rating"`
	RuntimeMinutes   int      `json:"runtime_minutes"`
	MetadataStatus   string   `json:"metadata_status"`
	TMDBID           int64    `json:"tmdb_id,omitempty"`
	CollectionTMDBID int64    `json:"collection_tmdb_id,omitempty"`
	CollectionName   string   `json:"collection_name,omitempty"`
	PosterURL        string   `json:"poster_url"`
	BackdropURL      string   `json:"backdrop_url"`
	LogoURL          string   `json:"logo_url"`
	EntityType       string   `json:"entity_type,omitempty"`
	SeriesID         string   `json:"series_id,omitempty"`
	SeasonCount      int      `json:"season_count,omitempty"`
	EpisodeCount     int      `json:"episode_count,omitempty"`
	PositionSeconds  float64  `json:"position_seconds,omitempty"`
	DurationSeconds  float64  `json:"duration_seconds,omitempty"`
	ProgressPercent  float64  `json:"progress_percent,omitempty"`
}

type StreamItem struct {
	Item
	Path string
}

type Service struct {
	db        *sql.DB
	lifecycle context.Context

	projectionMu         sync.Mutex
	projectionRefreshMu  sync.Mutex
	projectionRefreshing bool
}

func NewService(db *sql.DB) *Service { return NewServiceWithContext(context.Background(), db) }

func NewServiceWithContext(lifecycle context.Context, db *sql.DB) *Service {
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	return &Service{db: db, lifecycle: lifecycle}
}

func (s *Service) List(ctx context.Context, libraryID int64, query string, limit, offset int, allowedLibraryIDs []int64) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{}
	sqlText := `SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),COALESCE(mm.collection_tmdb_id,0),COALESCE(mm.collection_name,''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),'')
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.available=1 AND l.kind<>'music'`
	if libraryID > 0 {
		sqlText += ` AND m.library_id=?`
		args = append(args, libraryID)
	}
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		sqlText += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	if query != "" {
		sqlText += ` AND m.title LIKE ?`
		args = append(args, "%"+query+"%")
	}
	// Pull a little more than the requested page because multiple physical
	// sources can collapse into one logical title after the query.
	rawLimit := limit * 4
	if rawLimit < limit {
		rawLimit = limit
	}
	if rawLimit > 2000 {
		rawLimit = 2000
	}
	sqlText += ` ORDER BY m.title COLLATE NOCASE LIMIT ? OFFSET ?`
	args = append(args, rawLimit, offset)
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var v Item
		var genres string
		if err := rows.Scan(&v.ID, &v.LibraryID, &v.LibraryName, &v.Title, &v.Extension, &v.SizeBytes, &v.ModifiedUnix, &v.Available,
			&v.MediaType, &v.Year, &v.SeasonNumber, &v.EpisodeNumber, &v.Overview, &genres, &v.Rating, &v.RuntimeMinutes, &v.MetadataStatus, &v.TMDBID, &v.CollectionTMDBID, &v.CollectionName, &v.PosterURL, &v.BackdropURL, &v.LogoURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &v.Genres)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out = DedupeItems(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Service) GetStreamItem(ctx context.Context, id int64) (StreamItem, error) {
	var v StreamItem
	err := s.db.QueryRowContext(ctx, `SELECT id,library_id,title,path,extension,size_bytes,modified_unix,available FROM media WHERE id=?`, id).Scan(&v.ID, &v.LibraryID, &v.Title, &v.Path, &v.Extension, &v.SizeBytes, &v.ModifiedUnix, &v.Available)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamItem{}, sql.ErrNoRows
	}
	return v, err
}

func ContainsLibrary(ids []int64, id int64) bool {
	if ids == nil {
		return true
	}
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (s *Service) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE available=1`).Scan(&n)
	return n, err
}

func (s *Service) DeleteCatalogItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("media not found")
	}
	return nil
}
