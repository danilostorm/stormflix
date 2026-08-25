package media

import (
	"context"
	"database/sql"
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
}

func (s *Service) Versions(ctx context.Context, mediaID int64, allowedLibraryIDs []int64) ([]Version, error) {
	var title string
	var year int
	var tmdbID int64
	var sourceLibrary int64
	err := s.db.QueryRowContext(ctx, `SELECT m.title,m.library_id,COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0)
FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=? AND m.available=1`, mediaID).
		Scan(&title, &sourceLibrary, &year, &tmdbID)
	if err != nil {
		return nil, err
	}
	if allowedLibraryIDs != nil && !ContainsLibrary(allowedLibraryIDs, sourceLibrary) {
		return nil, sql.ErrNoRows
	}

	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.name,m.path,m.extension,m.size_bytes
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND ((? > 0 AND COALESCE(mm.tmdb_id,0)=?) OR (?=0 AND lower(m.title)=lower(?) AND COALESCE(mm.year,0)=?))
ORDER BY m.id`, tmdbID, tmdbID, tmdbID, title, year)
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
			return out[i].SizeBytes > out[j].SizeBytes
		}
		return qi > qj
	})
	return out, nil
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
