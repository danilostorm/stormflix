package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type CatalogProjectionStatus struct {
	SourceRevision int64  `json:"source_revision"`
	BuiltRevision  int64  `json:"built_revision"`
	EntityCount    int64  `json:"entity_count"`
	BuiltAt        string `json:"built_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	Refreshing     bool   `json:"refreshing"`
}

type projectionSource struct {
	Item
	ReleaseDate string
}

type projectionEntity struct {
	key         string
	item        Item
	releaseDate string
	members     []int64
}

func (s *Service) CatalogProjectionStatus(ctx context.Context) (CatalogProjectionStatus, error) {
	var status CatalogProjectionStatus
	var builtAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT source_revision,built_revision,built_at,last_error,(SELECT COUNT(*) FROM catalog_entities) FROM catalog_projection_state WHERE id=1`).
		Scan(&status.SourceRevision, &status.BuiltRevision, &builtAt, &status.LastError, &status.EntityCount)
	if err != nil {
		return status, err
	}
	if builtAt.Valid {
		status.BuiltAt = builtAt.String
	}
	s.projectionRefreshMu.Lock()
	status.Refreshing = s.projectionRefreshing
	s.projectionRefreshMu.Unlock()
	return status, nil
}

func (s *Service) ensureCatalogProjection(ctx context.Context) (CatalogProjectionStatus, error) {
	status, err := s.CatalogProjectionStatus(ctx)
	if err != nil {
		return status, err
	}
	if status.BuiltRevision >= status.SourceRevision {
		return status, nil
	}
	if status.EntityCount > 0 {
		s.startCatalogProjectionRefresh()
		return status, nil
	}
	if err := s.RebuildCatalogProjection(ctx); err != nil {
		return status, err
	}
	return s.CatalogProjectionStatus(ctx)
}

func (s *Service) startCatalogProjectionRefresh() {
	s.projectionRefreshMu.Lock()
	if s.projectionRefreshing {
		s.projectionRefreshMu.Unlock()
		return
	}
	s.projectionRefreshing = true
	s.projectionRefreshMu.Unlock()
	go func() {
		defer func() {
			s.projectionRefreshMu.Lock()
			s.projectionRefreshing = false
			s.projectionRefreshMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(s.lifecycle, 5*time.Minute)
		defer cancel()
		if err := s.gate.Wait(ctx, "catalog_projection", nil); err != nil {
			return
		}
		_ = s.RebuildCatalogProjection(ctx)
	}()
}

// RebuildCatalogProjection atomically swaps the complete read model. WAL
// readers continue seeing the previous committed projection until the new one
// is ready, so a metadata scan cannot leave Home half-populated.
func (s *Service) RebuildCatalogProjection(ctx context.Context) error {
	s.projectionMu.Lock()
	defer s.projectionMu.Unlock()

	status, err := s.CatalogProjectionStatus(ctx)
	if err != nil {
		return err
	}
	if status.BuiltRevision >= status.SourceRevision {
		return nil
	}
	sourceRevision := status.SourceRevision

	var sources []projectionSource
	var groups []SeriesDetail
	var sourceErr, groupErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sources, sourceErr = s.loadProjectionSources(ctx)
	}()
	go func() {
		defer wg.Done()
		groups, groupErr = s.seriesGroups(ctx, nil)
	}()
	wg.Wait()
	if sourceErr != nil {
		s.recordProjectionError(sourceErr)
		return sourceErr
	}
	if groupErr != nil {
		s.recordProjectionError(groupErr)
		return groupErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	entities := buildProjectionEntities(sources, groups)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_entity_members; DELETE FROM catalog_entity_genres; DELETE FROM catalog_entities;`); err != nil {
		return fmt.Errorf("clear catalog projection: %w", err)
	}
	entityStmt, err := tx.PrepareContext(ctx, `INSERT INTO catalog_entities(
entity_key,entity_type,representative_media_id,library_id,library_name,title,extension,size_bytes,modified_unix,media_type,year,overview,genres_json,rating,runtime_minutes,metadata_status,tmdb_id,collection_tmdb_id,collection_name,release_date,poster_url,backdrop_url,logo_url,series_id,season_count,episode_count)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer entityStmt.Close()
	memberStmt, err := tx.PrepareContext(ctx, `INSERT INTO catalog_entity_members(media_id,entity_key,library_id) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer memberStmt.Close()
	genreStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO catalog_entity_genres(entity_key,genre,library_id,modified_unix) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer genreStmt.Close()
	keys := make([]string, 0, len(entities))
	for key := range entities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entity := entities[key]
		genres, _ := json.Marshal(entity.item.Genres)
		if _, err := entityStmt.ExecContext(ctx,
			entity.key, entity.item.EntityType, entity.item.ID, entity.item.LibraryID, entity.item.LibraryName,
			entity.item.Title, entity.item.Extension, entity.item.SizeBytes, entity.item.ModifiedUnix, entity.item.MediaType,
			entity.item.Year, entity.item.Overview, string(genres), entity.item.Rating, entity.item.RuntimeMinutes,
			entity.item.MetadataStatus, entity.item.TMDBID, entity.item.CollectionTMDBID, entity.item.CollectionName,
			entity.releaseDate, entity.item.PosterURL, entity.item.BackdropURL, entity.item.LogoURL,
			entity.item.SeriesID, entity.item.SeasonCount, entity.item.EpisodeCount,
		); err != nil {
			return fmt.Errorf("insert catalog entity %s: %w", entity.key, err)
		}
		for _, mediaID := range entity.members {
			if _, err := memberStmt.ExecContext(ctx, mediaID, entity.key, entity.item.LibraryID); err != nil {
				return fmt.Errorf("insert catalog member %d: %w", mediaID, err)
			}
		}
		for _, genre := range entity.item.Genres {
			genre = strings.TrimSpace(genre)
			if genre == "" {
				continue
			}
			if _, err := genreStmt.ExecContext(ctx, entity.key, genre, entity.item.LibraryID, entity.item.ModifiedUnix); err != nil {
				return fmt.Errorf("insert catalog genre %s/%s: %w", entity.key, genre, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE catalog_projection_state SET built_revision=?,built_at=CURRENT_TIMESTAMP,last_error='' WHERE id=1`, sourceRevision); err != nil {
		return fmt.Errorf("finish catalog projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit catalog projection: %w", err)
	}
	return nil
}

func (s *Service) recordProjectionError(cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	message := cause.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE catalog_projection_state SET last_error=? WHERE id=1`, message)
}

func buildProjectionEntities(sources []projectionSource, groups []SeriesDetail) map[string]*projectionEntity {
	entities := map[string]*projectionEntity{}
	byID := make(map[int64]projectionSource, len(sources))
	seriesMembers := map[int64]bool{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	for _, group := range groups {
		card := seriesToItem(group.SeriesSummary)
		representative := byID[card.ID]
		card.Extension = representative.Extension
		card.SizeBytes = representative.SizeBytes
		card.RuntimeMinutes = representative.RuntimeMinutes
		card.MetadataStatus = representative.MetadataStatus
		card.TMDBID = representative.TMDBID
		card.CollectionTMDBID = representative.CollectionTMDBID
		card.CollectionName = representative.CollectionName
		entity := &projectionEntity{key: "series:" + group.ID, item: card, releaseDate: representative.ReleaseDate}
		for _, season := range group.Seasons {
			for _, episode := range season.Episodes {
				entity.members = append(entity.members, episode.ID)
				seriesMembers[episode.ID] = true
			}
		}
		entities[entity.key] = entity
	}

	for _, source := range sources {
		if seriesMembers[source.ID] || isEpisodeItem(source.Item) {
			continue
		}
		logical := logicalItemKey(source.Item)
		if logical == "" {
			logical = fmt.Sprintf("id:%d", source.ID)
		}
		// Keep a representative inside each library. A global representative
		// could point a restricted profile at a physical file it cannot access.
		key := fmt.Sprintf("media:lib%d:%s", source.LibraryID, logical)
		entity := entities[key]
		if entity == nil {
			item := source.Item
			item.EntityType = "media"
			entities[key] = &projectionEntity{key: key, item: item, releaseDate: source.ReleaseDate, members: []int64{source.ID}}
			continue
		}
		entity.members = append(entity.members, source.ID)
		modified := entity.item.ModifiedUnix
		if source.ModifiedUnix > modified {
			modified = source.ModifiedUnix
		}
		if itemPresentationScore(source.Item) > itemPresentationScore(entity.item) {
			entity.item = source.Item
			entity.item.EntityType = "media"
			entity.releaseDate = source.ReleaseDate
		}
		entity.item.ModifiedUnix = modified
	}
	return entities
}

func (s *Service) loadProjectionSources(ctx context.Context) ([]projectionSource, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked_art AS (
  SELECT media_id,kind,public_url,
         ROW_NUMBER() OVER (PARTITION BY media_id,kind ORDER BY score DESC,id DESC) AS rank
  FROM media_artwork WHERE selected=1
), selected_art AS (
  SELECT media_id,
         MAX(CASE WHEN kind='poster' THEN public_url ELSE '' END) AS poster_url,
         MAX(CASE WHEN kind='backdrop' THEN public_url ELSE '' END) AS backdrop_url,
         MAX(CASE WHEN kind='logo' THEN public_url ELSE '' END) AS logo_url
  FROM ranked_art WHERE rank=1 GROUP BY media_id
)
SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),COALESCE(mm.collection_tmdb_id,0),COALESCE(mm.collection_name,''),
COALESCE(a.poster_url,''),COALESCE(a.backdrop_url,''),COALESCE(a.logo_url,''),COALESCE(mm.release_date,'')
FROM media m
JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN selected_art a ON a.media_id=m.id
WHERE m.available=1 AND l.enabled=1 AND l.kind<>'music'
ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []projectionSource{}
	for rows.Next() {
		var source projectionSource
		var genres string
		if err := rows.Scan(&source.ID, &source.LibraryID, &source.LibraryName, &source.Title, &source.Extension, &source.SizeBytes, &source.ModifiedUnix, &source.Available,
			&source.MediaType, &source.Year, &source.SeasonNumber, &source.EpisodeNumber, &source.Overview, &genres, &source.Rating, &source.RuntimeMinutes, &source.MetadataStatus, &source.TMDBID, &source.CollectionTMDBID, &source.CollectionName,
			&source.PosterURL, &source.BackdropURL, &source.LogoURL, &source.ReleaseDate); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &source.Genres)
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *Service) catalogEntities(ctx context.Context, allowedLibraryIDs []int64) ([]Item, error) {
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	query := catalogEntitySelect + ` WHERE 1=1`
	args := []any{}
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	return s.queryCatalogEntities(ctx, query, args...)
}

// CatalogList pages the logical catalog projection. Unlike List, it never
// expands a show into every episode or a title into every physical source.
// Category/gallery endpoints use it to keep response time proportional to the
// number of cards returned instead of the number of files on disk.
func (s *Service) CatalogList(ctx context.Context, libraryID int64, queryText string, limit, offset int, allowedLibraryIDs []int64) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	query := catalogEntitySelect + ` WHERE 1=1`
	args := []any{}
	if libraryID > 0 {
		query += ` AND ce.library_id=?`
		args = append(args, libraryID)
	}
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	if queryText = strings.TrimSpace(queryText); queryText != "" {
		query += ` AND ce.title LIKE ?`
		args = append(args, "%"+queryText+"%")
	}
	query += ` ORDER BY ce.title COLLATE NOCASE,ce.entity_key LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	return s.queryCatalogEntities(ctx, query, args...)
}

func (s *Service) projectedSeries(ctx context.Context, allowedLibraryIDs []int64, queryText string) ([]SeriesSummary, error) {
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []SeriesSummary{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	query := catalogEntitySelect + ` WHERE ce.entity_type='series'`
	args := []any{}
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	if queryText = strings.TrimSpace(queryText); queryText != "" {
		query += ` AND ce.title LIKE ?`
		args = append(args, "%"+queryText+"%")
	}
	query += ` ORDER BY ce.modified_unix DESC,ce.title COLLATE NOCASE,ce.entity_key`
	items, err := s.queryCatalogEntities(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := make([]SeriesSummary, 0, len(items))
	for _, item := range items {
		out = append(out, SeriesSummary{
			ID: item.SeriesID, EntityType: "series", RepresentativeMediaID: item.ID,
			LibraryID: item.LibraryID, LibraryName: item.LibraryName, MediaType: item.MediaType,
			Title: item.Title, Year: item.Year, Overview: item.Overview, Genres: append([]string(nil), item.Genres...),
			Rating: item.Rating, PosterURL: item.PosterURL, BackdropURL: item.BackdropURL,
			LogoURL: item.LogoURL, ModifiedUnix: item.ModifiedUnix,
			SeasonCount: item.SeasonCount, EpisodeCount: item.EpisodeCount,
		})
	}
	return out, nil
}

func (s *Service) relatedCatalogCandidates(ctx context.Context, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	query := catalogEntitySelect + ` WHERE 1=1`
	args := []any{}
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	query += ` ORDER BY ce.rating DESC,ce.modified_unix DESC LIMIT ?`
	args = append(args, limit)
	return s.queryCatalogEntities(ctx, query, args...)
}

const catalogEntitySelect = `SELECT ce.representative_media_id,ce.library_id,ce.library_name,ce.title,ce.extension,ce.size_bytes,ce.modified_unix,
ce.media_type,ce.year,ce.overview,ce.genres_json,ce.rating,ce.runtime_minutes,ce.metadata_status,ce.tmdb_id,ce.collection_tmdb_id,ce.collection_name,
ce.poster_url,ce.backdrop_url,ce.logo_url,ce.entity_type,ce.series_id,ce.season_count,ce.episode_count FROM catalog_entities ce`

func appendAllowedEntityLibraries(query string, args []any, allowedLibraryIDs []int64) (string, []any) {
	if allowedLibraryIDs == nil {
		return query, args
	}
	marks := make([]string, len(allowedLibraryIDs))
	for i, id := range allowedLibraryIDs {
		marks[i] = "?"
		args = append(args, id)
	}
	return query + ` AND ce.library_id IN (` + strings.Join(marks, ",") + `)`, args
}

func (s *Service) queryCatalogEntities(ctx context.Context, query string, args ...any) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var item Item
		var genres string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix,
			&item.MediaType, &item.Year, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID, &item.CollectionTMDBID, &item.CollectionName,
			&item.PosterURL, &item.BackdropURL, &item.LogoURL, &item.EntityType, &item.SeriesID, &item.SeasonCount, &item.EpisodeCount); err != nil {
			return nil, err
		}
		item.Available = true
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) projectedItemsFromMediaIDs(ctx context.Context, ids []int64, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if len(ids) == 0 || allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	marks := make([]string, len(ids))
	args := make([]any, 0, len(ids)+len(allowedLibraryIDs))
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	query := catalogEntitySelect + ` JOIN catalog_entity_members cem ON cem.entity_key=ce.entity_key WHERE cem.media_id IN (` + strings.Join(marks, ",") + `)`
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	items, err := s.queryCatalogEntities(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	byRepresentative := map[int64]Item{}
	byKey := map[string]Item{}
	for _, item := range items {
		byRepresentative[item.ID] = item
		key := fmt.Sprintf("%d:%s:%s", item.LibraryID, item.EntityType, item.SeriesID)
		if item.SeriesID == "" {
			key = fmt.Sprintf("%d:media:%d", item.LibraryID, item.ID)
		}
		byKey[key] = item
	}
	// Resolve media ID to entity in one compact mapping query, then restore the
	// ranking order returned by the discovery query.
	mapRows, err := s.db.QueryContext(ctx, `SELECT cem.media_id,ce.representative_media_id,ce.library_id,ce.entity_type,ce.series_id FROM catalog_entity_members cem JOIN catalog_entities ce ON ce.entity_key=cem.entity_key WHERE cem.media_id IN (`+strings.Join(marks, ",")+`)`, toAnyIDs(ids)...)
	if err != nil {
		return nil, err
	}
	resolved := map[int64]Item{}
	for mapRows.Next() {
		var mediaID, representative, libraryID int64
		var entityType, seriesID string
		if err := mapRows.Scan(&mediaID, &representative, &libraryID, &entityType, &seriesID); err != nil {
			_ = mapRows.Close()
			return nil, err
		}
		key := fmt.Sprintf("%d:%s:%s", libraryID, entityType, seriesID)
		if seriesID == "" {
			key = fmt.Sprintf("%d:media:%d", libraryID, representative)
		}
		if item, ok := byKey[key]; ok {
			resolved[mediaID] = item
		} else if item, ok := byRepresentative[representative]; ok {
			resolved[mediaID] = item
		}
	}
	if err := mapRows.Close(); err != nil {
		return nil, err
	}
	out := []Item{}
	seen := map[string]bool{}
	for _, id := range ids {
		item, ok := resolved[id]
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d:%s:%d:%s", item.LibraryID, item.EntityType, item.ID, item.SeriesID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func toAnyIDs(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

func (s *Service) projectedReleases(ctx context.Context, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return nil, err
	}
	query := catalogEntitySelect + ` WHERE ce.release_date<>'' AND ce.release_date<=date('now','+30 days') AND ce.release_date>=date('now','-730 days')`
	args := []any{}
	query, args = appendAllowedEntityLibraries(query, args, allowedLibraryIDs)
	query += ` ORDER BY ce.release_date DESC,ce.entity_key LIMIT ?`
	args = append(args, limit)
	return s.queryCatalogEntities(ctx, query, args...)
}

// recentEpisodesForHome deliberately remains a file-level query: this is the
// one Home rail where users want individual episodes. It only reads the newest
// candidates and joins selected artwork once, instead of rebuilding every
// series and sorting its complete episode list.
func (s *Service) recentEpisodesForHome(ctx context.Context, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	if limit <= 0 {
		return []Item{}, nil
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{}
	query := `WITH ranked_art AS (
  SELECT media_id,kind,public_url,
         ROW_NUMBER() OVER (PARTITION BY media_id,kind ORDER BY score DESC,id DESC) AS rank
  FROM media_artwork WHERE selected=1
), selected_art AS (
  SELECT media_id,
         MAX(CASE WHEN kind='poster' THEN public_url ELSE '' END) AS poster_url,
         MAX(CASE WHEN kind='backdrop' THEN public_url ELSE '' END) AS backdrop_url,
         MAX(CASE WHEN kind='logo' THEN public_url ELSE '' END) AS logo_url
  FROM ranked_art WHERE rank=1 GROUP BY media_id
)
SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(NULLIF(mm.media_type,''),'series'),COALESCE(mm.year,0),
CASE WHEN COALESCE(si.season_number,0)>0 THEN si.season_number ELSE COALESCE(mm.season_number,0) END,
CASE WHEN COALESCE(si.episode_number,0)>0 THEN si.episode_number ELSE COALESCE(mm.episode_number,0) END,
COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),
COALESCE(a.poster_url,''),COALESCE(a.backdrop_url,''),COALESCE(a.logo_url,'')
FROM media m
JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN media_series_identity si ON si.media_id=m.id
LEFT JOIN selected_art a ON a.media_id=m.id
WHERE m.available=1 AND l.enabled=1
  AND (COALESCE(si.episode_number,0)>0 OR COALESCE(mm.episode_number,0)>0)`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		query += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	query += ` ORDER BY m.modified_unix DESC,m.id DESC LIMIT ?`
	args = append(args, limit*3)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var item Item
		var genres string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available,
			&item.MediaType, &item.Year, &item.SeasonNumber, &item.EpisodeNumber, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID,
			&item.PosterURL, &item.BackdropURL, &item.LogoURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items = DedupeItems(items)
	return capItems(items, limit), nil
}
