package games

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxGameArtworkBytes int64 = 25 << 20

func (s *Service) storeMetadataCandidate(ctx context.Context, game metadataGameRow, candidate gameMetadataCandidate) error {
	var locked bool
	_ = s.db.QueryRowContext(ctx, `SELECT metadata_locked FROM game_metadata WHERE game_id=?`, game.ID).Scan(&locked)
	if locked {
		return nil
	}

	coverPath := ""
	if candidate.CoverURL != "" {
		if path, err := s.downloadGameArtwork(ctx, game, candidate.CoverURL); err == nil {
			coverPath = path
		}
	}
	genres, _ := json.Marshal(candidate.Genres)
	developers, _ := json.Marshal(candidate.Developers)
	publishers, _ := json.Marshal(candidate.Publishers)
	screenshots, _ := json.Marshal(candidate.Screenshots)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO game_metadata(game_id,primary_provider,primary_id,igdb_id,steamgriddb_id,mobygames_id,genres_json,developers_json,publishers_json,screenshots_json,community_rating,last_error,refreshed_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,'',CURRENT_TIMESTAMP)
ON CONFLICT(game_id) DO UPDATE SET
 primary_provider=excluded.primary_provider,primary_id=excluded.primary_id,
 igdb_id=CASE WHEN excluded.igdb_id<>'' THEN excluded.igdb_id ELSE game_metadata.igdb_id END,
 steamgriddb_id=CASE WHEN excluded.steamgriddb_id<>'' THEN excluded.steamgriddb_id ELSE game_metadata.steamgriddb_id END,
 mobygames_id=CASE WHEN excluded.mobygames_id<>'' THEN excluded.mobygames_id ELSE game_metadata.mobygames_id END,
 genres_json=excluded.genres_json,developers_json=excluded.developers_json,publishers_json=excluded.publishers_json,
 screenshots_json=excluded.screenshots_json,community_rating=excluded.community_rating,last_error='',refreshed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
		game.ID, candidate.Provider, candidate.ProviderID,
		providerID(candidate, "igdb"), candidate.SteamGridDBID, providerID(candidate, "mobygames"),
		string(genres), string(developers), string(publishers), string(screenshots), candidate.Rating)
	if err != nil {
		return err
	}

	title := game.Title
	if titleScore(game.Title, candidate.Title) >= .74 && strings.TrimSpace(candidate.Title) != "" {
		title = strings.TrimSpace(candidate.Title)
	}
	if coverPath != "" {
		_, err = tx.ExecContext(ctx, `UPDATE games SET title=?,sort_title=?,overview=?,release_year=?,cover_path=?,metadata_provider=?,metadata_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, strings.ToLower(title), candidate.Overview, candidate.ReleaseYear, coverPath, candidate.Provider, candidate.ProviderID, game.ID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE games SET title=?,sort_title=?,overview=?,release_year=?,metadata_provider=?,metadata_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, strings.ToLower(title), candidate.Overview, candidate.ReleaseYear, candidate.Provider, candidate.ProviderID, game.ID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func providerID(candidate gameMetadataCandidate, provider string) string {
	if candidate.Provider == provider {
		return candidate.ProviderID
	}
	return ""
}

func (s *Service) persistProviderIDs(ctx context.Context, gameID int64, ids map[string]string) error {
	if gameID <= 0 || len(ids) == 0 {
		return nil
	}
	clean := func(key string) string { return strings.TrimSpace(ids[key]) }
	_, err := s.db.ExecContext(ctx, `
INSERT INTO game_metadata(
 game_id,igdb_id,steamgriddb_id,mobygames_id,screenscraper_id,retroachievements_id,
 launchbox_id,hasheous_id,thegamesdb_id,flashpoint_id,hltb_id,demozoo_id,pouet_id,csdb_id,libretro_id
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(game_id) DO UPDATE SET
 igdb_id=CASE WHEN excluded.igdb_id<>'' THEN excluded.igdb_id ELSE game_metadata.igdb_id END,
 steamgriddb_id=CASE WHEN excluded.steamgriddb_id<>'' THEN excluded.steamgriddb_id ELSE game_metadata.steamgriddb_id END,
 mobygames_id=CASE WHEN excluded.mobygames_id<>'' THEN excluded.mobygames_id ELSE game_metadata.mobygames_id END,
 screenscraper_id=CASE WHEN excluded.screenscraper_id<>'' THEN excluded.screenscraper_id ELSE game_metadata.screenscraper_id END,
 retroachievements_id=CASE WHEN excluded.retroachievements_id<>'' THEN excluded.retroachievements_id ELSE game_metadata.retroachievements_id END,
 launchbox_id=CASE WHEN excluded.launchbox_id<>'' THEN excluded.launchbox_id ELSE game_metadata.launchbox_id END,
 hasheous_id=CASE WHEN excluded.hasheous_id<>'' THEN excluded.hasheous_id ELSE game_metadata.hasheous_id END,
 thegamesdb_id=CASE WHEN excluded.thegamesdb_id<>'' THEN excluded.thegamesdb_id ELSE game_metadata.thegamesdb_id END,
 flashpoint_id=CASE WHEN excluded.flashpoint_id<>'' THEN excluded.flashpoint_id ELSE game_metadata.flashpoint_id END,
 hltb_id=CASE WHEN excluded.hltb_id<>'' THEN excluded.hltb_id ELSE game_metadata.hltb_id END,
 demozoo_id=CASE WHEN excluded.demozoo_id<>'' THEN excluded.demozoo_id ELSE game_metadata.demozoo_id END,
 pouet_id=CASE WHEN excluded.pouet_id<>'' THEN excluded.pouet_id ELSE game_metadata.pouet_id END,
 csdb_id=CASE WHEN excluded.csdb_id<>'' THEN excluded.csdb_id ELSE game_metadata.csdb_id END,
 libretro_id=CASE WHEN excluded.libretro_id<>'' THEN excluded.libretro_id ELSE game_metadata.libretro_id END,
 updated_at=CURRENT_TIMESTAMP`,
		gameID,
		clean("igdb"), clean("steamgriddb"), clean("mobygames"), clean("screenscraper"), clean("retroachievements"),
		clean("launchbox"), clean("hasheous"), clean("thegamesdb"), clean("flashpoint"), clean("hltb"), clean("demozoo"), clean("pouet"), clean("csdb"), clean("libretro"),
	)
	return err
}

func (s *Service) storeMetadataError(ctx context.Context, gameID int64, cause error) error {
	message := cause.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO game_metadata(game_id,last_error,refreshed_at) VALUES(?,?,CURRENT_TIMESTAMP) ON CONFLICT(game_id) DO UPDATE SET last_error=excluded.last_error,refreshed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, gameID, message)
	return err
}

func (s *Service) finishMetadataJob(jobID int64, processed, matched, failed int, status, message string) {
	if len(message) > 500 {
		message = message[:500]
	}
	_, _ = s.db.Exec(`UPDATE game_metadata_jobs SET status=?,progress=100,processed=?,matched=?,failed=?,message=?,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, processed, matched, failed, message, jobID)
}

func (s *Service) downloadGameArtwork(ctx context.Context, game metadataGameRow, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "StormFlix/0.25 Games")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("artwork HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("artwork content type %q is not an image", contentType)
	}
	ext := artworkExtension(rawURL, contentType)
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir, "game-metadata", game.Platform, game.Hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, old := range []string{"cover.jpg", "cover.png", "cover.webp"} {
		_ = os.Remove(filepath.Join(dir, old))
	}
	path := filepath.Join(dir, "cover"+ext)
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxGameArtworkBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n <= 0 || n > maxGameArtworkBytes {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return "", errors.New("invalid game artwork size")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func artworkExtension(rawURL, contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(filepath.Ext(parsed.Path))
		if ext == ".png" || ext == ".webp" || ext == ".jpg" || ext == ".jpeg" {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	return ".jpg"
}
