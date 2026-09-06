package media

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const deviceVersionBatchSize = 250

type versionIdentity struct {
	title         string
	year          int
	tmdbID        int64
	mediaType     string
	seasonNumber  int
	episodeNumber int
}

// VersionCandidates returns every physical version that belongs to each
// requested logical title/episode. It performs two bounded queries per batch
// instead of calling Versions once per Home card, preserving the Home latency
// budget while allowing the HTTP layer to choose a device-compatible source.
func (s *Service) VersionCandidates(ctx context.Context, mediaIDs []int64, allowedLibraryIDs []int64) (map[int64][]Version, error) {
	out := map[int64][]Version{}
	ids := uniquePositiveIDs(mediaIDs)
	if len(ids) == 0 || allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return out, nil
	}
	for start := 0; start < len(ids); start += deviceVersionBatchSize {
		end := start + deviceVersionBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := s.loadVersionCandidateBatch(ctx, ids[start:end], allowedLibraryIDs, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Service) loadVersionCandidateBatch(ctx context.Context, ids, allowedLibraryIDs []int64, out map[int64][]Version) error {
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	requestedQuery := `SELECT m.id,m.title,COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0),COALESCE(mm.media_type,''),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0)
FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.available=1 AND m.id IN (` + marks + `)`
	if allowedLibraryIDs != nil {
		requestedQuery += " AND m.library_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(allowedLibraryIDs)), ",") + ")"
		for _, id := range allowedLibraryIDs {
			args = append(args, id)
		}
	}
	rows, err := s.db.QueryContext(ctx, requestedQuery, args...)
	if err != nil {
		return err
	}
	requested := map[int64]versionIdentity{}
	tmdbIDs := map[int64]bool{}
	titles := map[string]bool{}
	for rows.Next() {
		var id int64
		var identity versionIdentity
		if err := rows.Scan(&id, &identity.title, &identity.year, &identity.tmdbID, &identity.mediaType, &identity.seasonNumber, &identity.episodeNumber); err != nil {
			rows.Close()
			return err
		}
		if identity.tmdbID > 0 {
			tmdbIDs[identity.tmdbID] = true
		} else if identity.title != "" {
			titles[sqliteLowerTitle(identity.title)] = true
		}
		requested[id] = normalizeVersionIdentity(identity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(requested) == 0 {
		return nil
	}

	query := `SELECT m.id,m.library_id,l.name,m.path,m.extension,m.size_bytes,
COALESCE(mt.width,0),COALESCE(mt.height,0),COALESCE(mt.hdr,''),COALESCE(mt.video_codec,''),CASE WHEN mt.source_modified_unix=m.modified_unix THEN mt.status ELSE 'pending' END,
COALESCE(mm.year,0),COALESCE(mm.tmdb_id,0),COALESCE(mm.media_type,''),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),m.title
FROM media m JOIN libraries l ON l.id=m.library_id
LEFT JOIN media_metadata mm ON mm.media_id=m.id
LEFT JOIN media_technical mt ON mt.media_id=m.id
WHERE m.available=1 AND (`
	queryArgs := []any{}
	conditions := []string{}
	if len(tmdbIDs) > 0 {
		values := sortedInt64Keys(tmdbIDs)
		conditions = append(conditions, "mm.tmdb_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+")")
		for _, value := range values {
			queryArgs = append(queryArgs, value)
		}
	}
	if len(titles) > 0 {
		values := sortedStringKeys(titles)
		conditions = append(conditions, "lower(trim(m.title)) IN ("+strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")+")")
		for _, value := range values {
			queryArgs = append(queryArgs, value)
		}
	}
	if len(conditions) == 0 {
		return nil
	}
	query += strings.Join(conditions, " OR ") + ")"
	if allowedLibraryIDs != nil {
		query += " AND m.library_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(allowedLibraryIDs)), ",") + ")"
		for _, id := range allowedLibraryIDs {
			queryArgs = append(queryArgs, id)
		}
	}
	query += " ORDER BY m.id"

	rows, err = s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()
	tmdbCandidates := map[string][]Version{}
	titleCandidates := map[string][]Version{}
	for rows.Next() {
		var candidate Version
		var path string
		var identity versionIdentity
		if err := rows.Scan(&candidate.ID, &candidate.LibraryID, &candidate.LibraryName, &path, &candidate.Extension, &candidate.SizeBytes,
			&candidate.Width, &candidate.Height, &candidate.HDR, &candidate.VideoCodec, &candidate.TechnicalStatus,
			&identity.year, &identity.tmdbID, &identity.mediaType, &identity.seasonNumber, &identity.episodeNumber, &identity.title); err != nil {
			return err
		}
		identity = normalizeVersionIdentity(identity)
		candidate.Label = qualityFromDimensions(candidate.Width, candidate.Height, path)
		if identity.tmdbID > 0 {
			tmdbCandidates[tmdbVersionKey(identity)] = append(tmdbCandidates[tmdbVersionKey(identity)], candidate)
		}
		titleCandidates[titleVersionKey(identity)] = append(titleCandidates[titleVersionKey(identity)], candidate)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for id, identity := range requested {
		if identity.tmdbID > 0 {
			out[id] = append([]Version(nil), tmdbCandidates[tmdbVersionKey(identity)]...)
		} else {
			out[id] = append([]Version(nil), titleCandidates[titleVersionKey(identity)]...)
		}
		sortVersions(out[id])
	}
	return nil
}

func normalizeVersionIdentity(identity versionIdentity) versionIdentity {
	identity.title = strings.ToLower(strings.TrimSpace(identity.title))
	identity.mediaType = strings.ToLower(strings.TrimSpace(identity.mediaType))
	return identity
}

// SQLite's built-in lower() folds ASCII only. Keep query keys consistent so
// uppercase accented titles without provider metadata still find themselves.
func sqliteLowerTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.TrimSpace(title))
}

func tmdbVersionKey(identity versionIdentity) string {
	return fmt.Sprintf("%d:%s:%d:%d", identity.tmdbID, identity.mediaType, identity.seasonNumber, identity.episodeNumber)
}

func titleVersionKey(identity versionIdentity) string {
	return fmt.Sprintf("%s:%d:%s:%d:%d", identity.title, identity.year, identity.mediaType, identity.seasonNumber, identity.episodeNumber)
}

func sortVersions(values []Version) {
	sort.SliceStable(values, func(i, j int) bool {
		qi, qj := qualityRank(values[i].Label), qualityRank(values[j].Label)
		if qi == qj {
			if values[i].SourceIndex == values[j].SourceIndex {
				return values[i].SizeBytes > values[j].SizeBytes
			}
			return values[i].SourceIndex < values[j].SourceIndex
		}
		return qi > qj
	})
}

func uniquePositiveIDs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sortedInt64Keys(values map[int64]bool) []int64 {
	out := make([]int64, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedStringKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
