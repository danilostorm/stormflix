package media

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	stormdb "github.com/danilostorm/stormflix/internal/database"
)

const (
	homeItemsPerRow = 24
	homeLibraryRows = 12
	homeGenreRows   = 8
)

type homeAssociation struct {
	rowID     string
	rowTitle  string
	rowOrder  int
	itemOrder int
	entityKey string
}

// homeQueryV2 keeps cold Home work bounded. It asks SQLite for only the cards
// that can actually be rendered instead of loading the entire logical catalog
// into Go and sorting it on every cache miss.
func (s *Service) homeQueryV2(ctx context.Context, allowedLibraryIDs []int64, heroMode, serverName string, themeEnabled bool, themeVolume int, themeAutoplay bool) (HomeFeed, error) {
	feed := HomeFeed{Rows: []HomeRow{}, ServerName: serverName, ThemePreviewEnabled: themeEnabled, ThemePreviewVolume: themeVolume, ThemePreviewAutoplay: themeAutoplay}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return feed, nil
	}
	if _, err := s.ensureCatalogProjection(ctx); err != nil {
		return feed, err
	}

	started := time.Now()
	associations, err := s.homeBaseAssociations(ctx, allowedLibraryIDs)
	stormdb.ObserveQuery("home.base_rows", time.Since(started), err)
	if err != nil {
		return feed, err
	}
	started = time.Now()
	genres, genreErr := s.homeGenreAssociations(ctx, allowedLibraryIDs)
	stormdb.ObserveQuery("home.genre_rows", time.Since(started), genreErr)
	if genreErr == nil {
		associations = append(associations, genres...)
	}

	keys := make([]string, 0, len(associations))
	seenKeys := make(map[string]bool, len(associations))
	for _, association := range associations {
		if association.entityKey == "" || seenKeys[association.entityKey] {
			continue
		}
		seenKeys[association.entityKey] = true
		keys = append(keys, association.entityKey)
	}
	started = time.Now()
	items, err := s.catalogEntitiesByKeys(ctx, keys)
	stormdb.ObserveQuery("home.cards_batch", time.Since(started), err)
	if err != nil {
		return feed, err
	}

	byKey := make(map[string]Item, len(items))
	for key, item := range items {
		byKey[key] = item
	}
	rows := map[string]*HomeRow{}
	rowOrder := map[string]int{}
	var featured []Item
	for _, association := range associations {
		item, ok := byKey[association.entityKey]
		if !ok {
			continue
		}
		if association.rowID == "__featured" {
			featured = append(featured, item)
			continue
		}
		row := rows[association.rowID]
		if row == nil {
			row = &HomeRow{ID: association.rowID, Title: association.rowTitle, Items: []Item{}}
			rows[association.rowID] = row
			rowOrder[association.rowID] = association.rowOrder
		}
		row.Items = append(row.Items, item)
	}
	orderedIDs := make([]string, 0, len(rows))
	for id := range rows {
		orderedIDs = append(orderedIDs, id)
	}
	sortHomeRowIDs(orderedIDs, rowOrder)
	for _, id := range orderedIDs {
		feed.Rows = append(feed.Rows, *rows[id])
	}

	var heroPool []Item
	for _, row := range feed.Rows {
		if row.ID == "recent" {
			heroPool = row.Items
		}
		if heroMode == "top_rated" && row.ID == "top-rated" {
			heroPool = row.Items
		}
	}
	if heroMode == "featured" && len(featured) > 0 {
		feed.Hero = copyItem(featured[0])
	}
	if feed.Hero == nil && len(heroPool) > 0 {
		index := 0
		if heroMode == "random" && len(heroPool) > 1 {
			index = int(time.Now().Unix()/3600) % len(heroPool)
		}
		feed.Hero = copyItem(heroPool[index])
	}
	return feed, nil
}

func (s *Service) homeBaseAssociations(ctx context.Context, allowedLibraryIDs []int64) ([]homeAssociation, error) {
	filter, args := homeLibraryFilter(allowedLibraryIDs)
	query := fmt.Sprintf(`WITH permitted AS (
  SELECT ce.* FROM catalog_entities ce WHERE 1=1 %s
), ranked AS (
  SELECT entity_key,'__featured' row_id,'' row_title,0 row_order,
         ROW_NUMBER() OVER (ORDER BY (backdrop_url<>'') DESC,rating DESC,modified_unix DESC,entity_key) item_order
  FROM permitted
  UNION ALL
  SELECT entity_key,'series-recent','Séries adicionadas recentemente',10,
         ROW_NUMBER() OVER (ORDER BY modified_unix DESC,entity_key)
  FROM permitted WHERE entity_type='series'
  UNION ALL
  SELECT entity_key,'recent','Adicionados recentemente',20,
         ROW_NUMBER() OVER (ORDER BY modified_unix DESC,entity_key)
  FROM permitted
  UNION ALL
  SELECT entity_key,'top-rated','Mais bem avaliados',30,
         ROW_NUMBER() OVER (ORDER BY rating DESC,modified_unix DESC,entity_key)
  FROM permitted WHERE rating>0
), libraries AS (
  SELECT library_id,library_name,
         ROW_NUMBER() OVER (ORDER BY MAX(modified_unix) DESC,library_name COLLATE NOCASE,library_id) library_order
  FROM permitted GROUP BY library_id,library_name
), library_ranked AS (
  SELECT p.entity_key,'library-' || p.library_id row_id,p.library_name row_title,
         100+l.library_order row_order,
         ROW_NUMBER() OVER (PARTITION BY p.library_id ORDER BY p.modified_unix DESC,p.entity_key) item_order,
         l.library_order
  FROM permitted p JOIN libraries l ON l.library_id=p.library_id
)
SELECT row_id,row_title,row_order,item_order,entity_key FROM ranked
WHERE (row_id='__featured' AND item_order<=1) OR (row_id<>'__featured' AND item_order<=%d)
UNION ALL
SELECT row_id,row_title,row_order,item_order,entity_key FROM library_ranked
WHERE library_order<=%d AND item_order<=%d
ORDER BY row_order,item_order`, filter, homeItemsPerRow, homeLibraryRows, homeItemsPerRow)
	return queryHomeAssociations(ctx, s.db, query, args...)
}

func (s *Service) homeGenreAssociations(ctx context.Context, allowedLibraryIDs []int64) ([]homeAssociation, error) {
	filter, args := homeLibraryFilter(allowedLibraryIDs)
	filter = strings.ReplaceAll(filter, "ce.", "ceg.")
	query := `SELECT genre,COUNT(*) item_count
FROM catalog_entity_genres ceg WHERE 1=1 ` + filter + `
GROUP BY genre COLLATE NOCASE HAVING COUNT(*)>=2
ORDER BY item_count DESC,genre COLLATE NOCASE LIMIT ?`
	topArgs := append(append([]any(nil), args...), homeGenreRows)
	rows, err := s.db.QueryContext(ctx, query, topArgs...)
	if err != nil {
		return nil, err
	}
	topGenres := make([]string, 0, homeGenreRows)
	for rows.Next() {
		var genre string
		var count int
		if err := rows.Scan(&genre, &count); err != nil {
			rows.Close()
			return nil, err
		}
		topGenres = append(topGenres, genre)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Each top genre is an indexed, bounded probe. This avoids parsing every
	// genres_json value and sorting the full catalog on every cold Home request.
	out := make([]homeAssociation, 0, len(topGenres)*homeItemsPerRow)
	for genreOrder, genre := range topGenres {
		genreArgs := make([]any, 0, len(args)+2)
		genreArgs = append(genreArgs, genre)
		genreArgs = append(genreArgs, args...)
		genreArgs = append(genreArgs, homeItemsPerRow)
		genreRows, err := s.db.QueryContext(ctx, `SELECT entity_key
FROM catalog_entity_genres ceg
WHERE genre=? COLLATE NOCASE `+filter+`
ORDER BY modified_unix DESC,entity_key LIMIT ?`, genreArgs...)
		if err != nil {
			return nil, err
		}
		itemOrder := 0
		for genreRows.Next() {
			var entityKey string
			if err := genreRows.Scan(&entityKey); err != nil {
				genreRows.Close()
				return nil, err
			}
			itemOrder++
			out = append(out, homeAssociation{
				rowID: "genre-" + hex.EncodeToString([]byte(genre)), rowTitle: genre,
				rowOrder: 500 + genreOrder + 1, itemOrder: itemOrder, entityKey: entityKey,
			})
		}
		if err := genreRows.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func queryHomeAssociations(ctx context.Context, db queryContext, query string, args ...any) ([]homeAssociation, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []homeAssociation{}
	for rows.Next() {
		var association homeAssociation
		if err := rows.Scan(&association.rowID, &association.rowTitle, &association.rowOrder, &association.itemOrder, &association.entityKey); err != nil {
			return nil, err
		}
		out = append(out, association)
	}
	return out, rows.Err()
}

type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func homeLibraryFilter(ids []int64) (string, []any) {
	if ids == nil {
		return "", nil
	}
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	return "AND ce.library_id IN (" + strings.Join(marks, ",") + ")", args
}

func (s *Service) catalogEntitiesByKeys(ctx context.Context, keys []string) (map[string]Item, error) {
	out := make(map[string]Item, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	marks := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, key := range keys {
		marks[i], args[i] = "?", key
	}
	query := `SELECT ce.entity_key,` + strings.TrimPrefix(catalogEntitySelect, "SELECT ") + ` WHERE ce.entity_key IN (` + strings.Join(marks, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, genres string
		var item Item
		if err := rows.Scan(&key, &item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix,
			&item.MediaType, &item.Year, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID, &item.CollectionTMDBID, &item.CollectionName,
			&item.PosterURL, &item.BackdropURL, &item.LogoURL, &item.EntityType, &item.SeriesID, &item.SeasonCount, &item.EpisodeCount); err != nil {
			return nil, err
		}
		item.Available = true
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		out[key] = item
	}
	return out, rows.Err()
}

func sortHomeRowIDs(ids []string, order map[string]int) {
	sort.SliceStable(ids, func(i, j int) bool {
		if order[ids[i]] == order[ids[j]] {
			return ids[i] < ids[j]
		}
		return order[ids[i]] < order[ids[j]]
	})
}
