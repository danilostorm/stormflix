package media

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Version struct {
	ID          int64  `json:"id"`
	Label       string `json:"label"`
	Extension   string `json:"extension"`
	SizeBytes   int64  `json:"size_bytes"`
	LibraryID   int64  `json:"library_id"`
	LibraryName string `json:"library_name"`
	SourceID    int64  `json:"source_id,omitempty"`
	SourceIndex int    `json:"source_index,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
	ServerLabel string `json:"server_label,omitempty"`
}

func (s *Service) Versions(ctx context.Context, mediaID int64, allowedLibraryIDs []int64) ([]Version, error) {
	var title string
	var year int
	var tmdbID int64
	var mediaType string
	var seasonNumber, episodeNumber int
	var sourceLibrary int64
	err := s.db.QueryRowContext(ctx, `SELECT m.title,m.library_id,COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0),COALESCE(mm.media_type,''),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0)
FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&title, &sourceLibrary, &year, &tmdbID, &mediaType, &seasonNumber, &episodeNumber)
	if err != nil {
		return nil, err
	}
	if allowedLibraryIDs != nil && !ContainsLibrary(allowedLibraryIDs, sourceLibrary) {
		return nil, sql.ErrNoRows
	}

	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.name,m.path,m.extension,m.size_bytes
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND (
  (? > 0 AND COALESCE(mm.tmdb_id,0)=? AND COALESCE(mm.media_type,'')=? AND COALESCE(mm.season_number,0)=? AND COALESCE(mm.episode_number,0)=?)
  OR
  (?=0 AND lower(m.title)=lower(?) AND COALESCE(mm.year,0)=? AND COALESCE(mm.media_type,'')=? AND COALESCE(mm.season_number,0)=? AND COALESCE(mm.episode_number,0)=?)
)
ORDER BY m.id`, tmdbID, tmdbID, mediaType, seasonNumber, episodeNumber, tmdbID, title, year, mediaType, seasonNumber, episodeNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		var path string
		if err := rows.Scan(&v.ID, &v.LibraryID, &v.LibraryName, &path, &v.Extension, &v.SizeBytes); err != nil {
			return nil, err
		}
		if allowedLibraryIDs != nil && !ContainsLibrary(allowedLibraryIDs, v.LibraryID) {
			continue
		}
		v.Label = qualityFromPath(path)
		v.SourceID, v.SourceIndex, v.SourceLabel, v.SourceName = s.sourceForPath(ctx, v.LibraryID, path)
		if v.SourceIndex <= 0 {
			v.SourceIndex = 1
		}
		if strings.TrimSpace(v.SourceName) == "" {
			v.SourceName = v.LibraryName
		}
		v.ServerLabel = fmt.Sprintf("Servidor %d · %s", v.SourceIndex, v.SourceName)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []Version{}, nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		qi, qj := qualityRank(out[i].Label), qualityRank(out[j].Label)
		if qi == qj {
			if out[i].SourceIndex == out[j].SourceIndex {
				return out[i].SizeBytes > out[j].SizeBytes
			}
			return out[i].SourceIndex < out[j].SourceIndex
		}
		return qi > qj
	})
	return out, nil
}

func (s *Service) sourceForPath(ctx context.Context, libraryID int64, mediaPath string) (int64, int, string, string) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,path,label,sort_order FROM library_sources WHERE library_id=? AND enabled=1 ORDER BY sort_order,id`, libraryID)
	if err != nil {
		return 0, 1, "Origem 1", ""
	}
	defer rows.Close()
	cleanMedia := filepath.Clean(mediaPath)
	bestRoot := ""
	var bestID int64
	bestIndex := 1
	bestLabel := "Origem 1"
	for rows.Next() {
		var id int64
		var root, label string
		var order int
		if rows.Scan(&id, &root, &label, &order) != nil {
			continue
		}
		root = filepath.Clean(root)
		if !pathInside(cleanMedia, root) {
			continue
		}
		if len(root) > len(bestRoot) {
			bestRoot = root
			bestID = id
			bestIndex = order + 1
			bestLabel = label
		}
	}
	name := ""
	if bestRoot != "" {
		parent := filepath.Base(filepath.Dir(bestRoot))
		if parent != "." && parent != string(filepath.Separator) && parent != "media" {
			name = parent
		} else {
			name = filepath.Base(bestRoot)
		}
	}
	return bestID, bestIndex, bestLabel, name
}

func pathInside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func qualityFromPath(path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.Contains(p, "4320p") || strings.Contains(p, "8k"):
		return "8K"
	case strings.Contains(p, "2160p") || strings.Contains(p, "4k") || strings.Contains(p, "uhd"):
		return "4K"
	case strings.Contains(p, "1440p"):
		return "1440p"
	case strings.Contains(p, "1080p"):
		return "1080p"
	case strings.Contains(p, "720p"):
		return "720p"
	case strings.Contains(p, "480p"):
		return "480p"
	default:
		return "Original"
	}
}

func qualityRank(label string) int {
	switch label {
	case "8K":
		return 6
	case "4K":
		return 5
	case "1440p":
		return 4
	case "1080p":
		return 3
	case "720p":
		return 2
	case "480p":
		return 1
	default:
		return 0
	}
}
