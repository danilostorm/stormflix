package media

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

type Collection struct {
	TMDBID    int64  `json:"tmdb_id"`
	Name      string `json:"name"`
	ItemCount int    `json:"item_count"`
	Items     []Item `json:"items"`
}

// Collections returns only movie franchises represented by enough locally
// available logical titles. The allowed-library filter is applied in SQL so a
// collection can never reveal titles from a library the user cannot access.
func (s *Service) Collections(ctx context.Context, allowedLibraryIDs []int64, minimumSize int) ([]Collection, error) {
	if minimumSize < 2 {
		minimumSize = 2
	}
	if minimumSize > 20 {
		minimumSize = 20
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Collection{}, nil
	}

	args := []any{}
	q := `SELECT m.id,m.library_id,l.name,m.title,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),COALESCE(mm.tmdb_id,0),COALESCE(mm.collection_tmdb_id,0),COALESCE(mm.collection_name,''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),'')
FROM media m
JOIN libraries l ON l.id=m.library_id
JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND l.kind<>'music' AND mm.media_type='movie'
  AND mm.collection_tmdb_id>0 AND TRIM(mm.collection_name)<>''`
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		q += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	q += ` ORDER BY mm.collection_name COLLATE NOCASE,mm.collection_tmdb_id,mm.year,m.title COLLATE NOCASE,m.id`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := map[int64]*Collection{}
	order := []int64{}
	for rows.Next() {
		var item Item
		var genres string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available,
			&item.MediaType, &item.Year, &item.SeasonNumber, &item.EpisodeNumber, &item.Overview, &genres, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.TMDBID, &item.CollectionTMDBID, &item.CollectionName, &item.PosterURL, &item.BackdropURL, &item.LogoURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		group := groups[item.CollectionTMDBID]
		if group == nil {
			group = &Collection{TMDBID: item.CollectionTMDBID, Name: item.CollectionName, Items: []Item{}}
			groups[item.CollectionTMDBID] = group
			order = append(order, item.CollectionTMDBID)
		}
		group.Items = append(group.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Collection, 0, len(groups))
	for _, id := range order {
		group := groups[id]
		group.Items = DedupeItems(group.Items)
		sort.SliceStable(group.Items, func(i, j int) bool {
			if group.Items[i].Year != group.Items[j].Year {
				if group.Items[i].Year == 0 {
					return false
				}
				if group.Items[j].Year == 0 {
					return true
				}
				return group.Items[i].Year < group.Items[j].Year
			}
			return strings.ToLower(group.Items[i].Title) < strings.ToLower(group.Items[j].Title)
		})
		group.ItemCount = len(group.Items)
		if group.ItemCount >= minimumSize {
			out = append(out, *group)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
