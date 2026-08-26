package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type logicalCopy struct {
	ID       int64
	Key      string
	Artwork  int
}

func (s *server) consolidateLibraryCopies(w http.ResponseWriter, r *http.Request) {
	libraryID, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT m.id,m.title,COALESCE(mm.tmdb_id,0),COALESCE(mm.year,0),COALESCE(mm.media_type,''),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),
       (SELECT COUNT(*) FROM media_artwork a WHERE a.media_id=m.id)
FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id
WHERE m.library_id=? AND m.available=1
ORDER BY m.id`, libraryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	groups := map[string][]logicalCopy{}
	for rows.Next() {
		var id, tmdbID int64
		var title, mediaType string
		var year, season, episode, artwork int
		if err := rows.Scan(&id, &title, &tmdbID, &year, &mediaType, &season, &episode, &artwork); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		key := logicalCopyKey(tmdbID, title, year, mediaType, season, episode)
		groups[key] = append(groups[key], logicalCopy{ID: id, Key: key, Artwork: artwork})
	}
	if err := rows.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()

	duplicateGroups := 0
	copiesLinked := 0
	artworkRowsLinked := 0
	oldAssets := map[string]bool{}
	for _, copies := range groups {
		if len(copies) < 2 {
			continue
		}
		duplicateGroups++
		sort.SliceStable(copies, func(i, j int) bool {
			if copies[i].Artwork == copies[j].Artwork {
				return copies[i].ID < copies[j].ID
			}
			return copies[i].Artwork > copies[j].Artwork
		})
		canonical := copies[0]
		if canonical.Artwork == 0 {
			continue
		}
		for _, copy := range copies[1:] {
			assetRows, qerr := tx.QueryContext(r.Context(), `SELECT asset_path FROM media_artwork WHERE media_id=? AND asset_path<>''`, copy.ID)
			if qerr != nil {
				writeError(w, http.StatusInternalServerError, qerr)
				return
			}
			for assetRows.Next() {
				var path string
				if assetRows.Scan(&path) == nil && strings.TrimSpace(path) != "" {
					oldAssets[path] = true
				}
			}
			_ = assetRows.Close()

			if _, err := tx.ExecContext(r.Context(), `DELETE FROM media_artwork WHERE media_id=?`, copy.ID); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			res, err := tx.ExecContext(r.Context(), `
INSERT OR IGNORE INTO media_artwork(media_id,kind,provider,source_url,asset_path,public_url,language,score,selected,created_at,updated_at)
SELECT ?,kind,provider,source_url,asset_path,public_url,language,score,selected,created_at,CURRENT_TIMESTAMP
FROM media_artwork WHERE media_id=?`, copy.ID, canonical.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if n, _ := res.RowsAffected(); n > 0 {
				artworkRowsLinked += int(n)
			}
			copiesLinked++
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	removedFiles := 0
	var freedBytes int64
	for assetPath := range oldAssets {
		var refs int
		if err := s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media_artwork WHERE asset_path=?`, assetPath).Scan(&refs); err != nil || refs > 0 {
			continue
		}
		fullPath, err := s.assets.Resolve(assetPath)
		if err != nil {
			continue
		}
		if info, err := os.Stat(fullPath); err == nil {
			freedBytes += info.Size()
		}
		if err := os.Remove(fullPath); err == nil || os.IsNotExist(err) {
			removedFiles++
			_ = os.Remove(filepath.Dir(fullPath))
		}
	}
	uid := currentUser(r).ID
	s.admin.Log(r.Context(), "info", "library", "Logical media copies consolidated", &uid, fmt.Sprintf("library=%d groups=%d copies=%d freed=%d", libraryID, duplicateGroups, copiesLinked, freedBytes))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"duplicate_groups": duplicateGroups,
		"copies_linked": copiesLinked,
		"artwork_rows_linked": artworkRowsLinked,
		"removed_files": removedFiles,
		"freed_bytes": freedBytes,
	})
}

func logicalCopyKey(tmdbID int64, title string, year int, mediaType string, season, episode int) string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if tmdbID > 0 {
		return fmt.Sprintf("tmdb:%d:%s:s%d:e%d", tmdbID, mediaType, season, episode)
	}
	return fmt.Sprintf("title:%s:%d:%s:s%d:e%d", strings.ToLower(strings.Join(strings.Fields(title), " ")), year, mediaType, season, episode)
}
