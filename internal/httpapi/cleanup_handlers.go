package httpapi

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type cleanupReport struct {
	AssetFiles       int   `json:"asset_files"`
	AssetBytes       int64 `json:"asset_bytes"`
	OrphanAssetFiles int   `json:"orphan_asset_files"`
	OrphanAssetBytes int64 `json:"orphan_asset_bytes"`
	TempFiles        int   `json:"temp_files"`
	TempBytes        int64 `json:"temp_bytes"`
	ExpiredSessions  int64 `json:"expired_sessions"`
	UnavailableMedia int64 `json:"unavailable_media"`
	OldLogs          int64 `json:"old_logs"`
	DatabaseBytes    int64 `json:"database_bytes"`
}

type cleanupRequest struct {
	OrphanAssets      bool `json:"orphan_assets"`
	TempFiles         bool `json:"temp_files"`
	ExpiredSessions   bool `json:"expired_sessions"`
	UnavailableMedia  bool `json:"unavailable_media"`
	LogsOlderThanDays int  `json:"logs_older_than_days"`
	Vacuum            bool `json:"vacuum"`
}

func (s *server) cleanupStatus(w http.ResponseWriter, r *http.Request) {
	report, _, _, err := s.buildCleanupReport()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, report)
}

func (s *server) runCleanup(w http.ResponseWriter, r *http.Request) {
	var in cleanupRequest
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.LogsOlderThanDays < 0 {
		in.LogsOlderThanDays = 0
	}
	if in.LogsOlderThanDays > 3650 {
		in.LogsOlderThanDays = 3650
	}
	if in.UnavailableMedia {
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM media WHERE available=0`); err != nil {
			writeError(w, 500, err)
			return
		}
	}
	if in.ExpiredSessions {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE expires_at<=CURRENT_TIMESTAMP`)
	}
	if in.LogsOlderThanDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(in.LogsOlderThanDays) * 24 * time.Hour).Format("2006-01-02 15:04:05")
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM activity_logs WHERE created_at<?`, cutoff)
	}

	_, orphans, temps, err := s.buildCleanupReport()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	removedFiles := 0
	removedBytes := int64(0)
	if in.OrphanAssets {
		for path, size := range orphans {
			if err := os.Remove(path); err == nil || os.IsNotExist(err) {
				removedFiles++
				removedBytes += size
			}
		}
	}
	if in.TempFiles {
		for path, size := range temps {
			if err := os.Remove(path); err == nil || os.IsNotExist(err) {
				removedFiles++
				removedBytes += size
			}
		}
	}
	if in.Vacuum {
		if _, err := s.db.ExecContext(r.Context(), `VACUUM`); err != nil {
			writeError(w, 500, err)
			return
		}
	}
	report, _, _, err := s.buildCleanupReport()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "removed_files": removedFiles, "freed_bytes": removedBytes, "report": report})
}

func (s *server) buildCleanupReport() (cleanupReport, map[string]int64, map[string]int64, error) {
	var report cleanupReport
	referenced := map[string]bool{}
	rows, err := s.db.Query(`SELECT asset_path FROM media_artwork WHERE asset_path<>'' UNION SELECT asset_path FROM subtitles WHERE asset_path<>''`)
	if err != nil {
		return report, nil, nil, err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			return report, nil, nil, err
		}
		value = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(value), "/"))
		if value != "" {
			referenced[value] = true
		}
	}
	if err := rows.Close(); err != nil {
		return report, nil, nil, err
	}
	root, _ := s.assets.Snapshot()
	orphans := map[string]int64{}
	temps := map[string]int64{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		report.AssetFiles++
		report.AssetBytes += info.Size()
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".tmp") {
			report.TempFiles++
			report.TempBytes += info.Size()
			temps[path] = info.Size()
			return nil
		}
		if !referenced[rel] {
			report.OrphanAssetFiles++
			report.OrphanAssetBytes += info.Size()
			orphans[path] = info.Size()
		}
		return nil
	})
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE expires_at<=CURRENT_TIMESTAMP`).Scan(&report.ExpiredSessions)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM media WHERE available=0`).Scan(&report.UnavailableMedia)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM activity_logs WHERE created_at<datetime('now','-90 days')`).Scan(&report.OldLogs)
	if info, err := os.Stat(s.config.DatabasePath()); err == nil {
		report.DatabaseBytes = info.Size()
	}
	return report, orphans, temps, nil
}
