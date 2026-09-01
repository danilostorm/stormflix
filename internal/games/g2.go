package games

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	NostalgistVersion = "0.21.1"
	RetroArchBuild    = "v1.22.2"
	maxStateBytes     = int64(32 << 20)
	maxSRAMBytes      = int64(8 << 20)
	maxRuntimeBytes   = int64(128 << 20)
)

var (
	gameSaveMu    sync.Mutex
	gameRuntimeMu sync.Mutex
	playSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,96}$`)
	contentHashRE = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type SaveInfo struct {
	Kind      string `json:"kind"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

type SaveStatus struct {
	State SaveInfo `json:"state"`
	SRAM  SaveInfo `json:"sram"`
}

type PlayableGame struct {
	Game
	ROMName      string     `json:"rom_name"`
	ROMSizeBytes int64      `json:"rom_size_bytes"`
	Playable     bool       `json:"playable"`
	Core         string     `json:"core"`
	Saves        SaveStatus `json:"saves"`
}

type GameFile struct {
	Path         string
	Name         string
	Extension    string
	SizeBytes    int64
	ModifiedUnix int64
}

type SaveFile struct {
	Path string
	SaveInfo
}

type playableRecord struct {
	PlayableGame
	contentHash  string
	romPath      string
	romExtension string
	modifiedUnix int64
}

var coreByPlatform = map[string]string{
	"nes":     "fceumm",
	"snes":    "snes9x",
	"genesis": "genesis_plus_gx",
	"gb":      "mgba",
	"gbc":     "mgba",
	"gba":     "mgba",
}

var allowedRuntimeCores = map[string]bool{
	"fceumm": true, "snes9x": true, "genesis_plus_gx": true, "mgba": true,
}

func CoreForPlatform(platform string) string {
	return coreByPlatform[strings.ToLower(strings.TrimSpace(platform))]
}

func (s *Service) loadPlayable(ctx context.Context, id, profileID int64, allowed []int64) (playableRecord, error) {
	where, accessArgs := allowedClause("g.library_id", allowed)
	args := []any{profileID, id}
	args = append(args, accessArgs...)
	var rec playableRecord
	var cover string
	err := s.db.QueryRowContext(ctx, `
SELECT g.id,g.library_id,l.name,g.platform,g.title,g.overview,g.release_year,g.cover_path,
       COALESCE(ps.favorite,0),COALESCE(ps.play_seconds,0),COALESCE(ps.last_played_at,''),g.created_at,
       g.content_hash,
       COALESCE((SELECT gf.path FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1 ORDER BY gf.id LIMIT 1),''),
       COALESCE((SELECT gf.extension FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1 ORDER BY gf.id LIMIT 1),''),
       COALESCE((SELECT gf.size_bytes FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1 ORDER BY gf.id LIMIT 1),0),
       COALESCE((SELECT gf.modified_unix FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1 ORDER BY gf.id LIMIT 1),0)
FROM games g
JOIN libraries l ON l.id=g.library_id
LEFT JOIN game_profile_state ps ON ps.game_id=g.id AND ps.profile_id=?
WHERE g.id=? AND EXISTS(SELECT 1 FROM game_files gf WHERE gf.game_id=g.id AND gf.available=1)`+where,
		args...).Scan(
		&rec.ID, &rec.LibraryID, &rec.Library, &rec.Platform, &rec.Title, &rec.Overview, &rec.ReleaseYear, &cover,
		&rec.Favorite, &rec.PlaySeconds, &rec.LastPlayed, &rec.CreatedAt,
		&rec.contentHash, &rec.romPath, &rec.romExtension, &rec.ROMSizeBytes, &rec.modifiedUnix,
	)
	if err != nil {
		return playableRecord{}, err
	}
	if cover != "" {
		rec.CoverURL = fmt.Sprintf("/api/v1/games/%d/cover", rec.ID)
	}
	rec.ROMName = filepath.Base(rec.romPath)
	rec.Playable = rec.romPath != "" && rec.ROMSizeBytes > 0
	rec.Core = CoreForPlatform(rec.Platform)
	return rec, nil
}

// PlayDetail is the G2 detail lookup. Unlike the G1 compatibility Detail method,
// it queries by id directly and therefore remains correct for libraries with
// more than 500 games.
func (s *Service) PlayDetail(ctx context.Context, id, profileID int64, allowed []int64) (PlayableGame, error) {
	rec, err := s.loadPlayable(ctx, id, profileID, allowed)
	if err != nil {
		return PlayableGame{}, err
	}
	rec.Saves, _ = s.SaveStatus(ctx, profileID, id)
	return rec.PlayableGame, nil
}

func (s *Service) PlayableFile(ctx context.Context, id, profileID int64, allowed []int64) (GameFile, error) {
	rec, err := s.loadPlayable(ctx, id, profileID, allowed)
	if err != nil {
		return GameFile{}, err
	}
	if !rec.Playable || rec.Core == "" {
		return GameFile{}, errors.New("game is not playable by the G2 browser matrix")
	}
	info, err := os.Stat(rec.romPath)
	if err != nil || info.IsDir() {
		return GameFile{}, errors.New("ROM file is unavailable")
	}
	if info.Size() <= 0 || info.Size() > maxROMBytes {
		return GameFile{}, errors.New("ROM file size is outside the supported G2 cartridge limit")
	}
	return GameFile{Path: rec.romPath, Name: filepath.Base(rec.romPath), Extension: rec.romExtension, SizeBytes: info.Size(), ModifiedUnix: info.ModTime().Unix()}, nil
}

func (s *Service) dataDir(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err != nil {
			return "", err
		}
		if name == "main" && strings.TrimSpace(path) != "" {
			return filepath.Dir(path), nil
		}
	}
	return "", errors.New("StormFlix data directory is unavailable")
}

func (s *Service) savePath(ctx context.Context, profileID, gameID int64, kind string, slot int) (string, error) {
	if profileID <= 0 || gameID <= 0 {
		return "", errors.New("profile and game are required")
	}
	if kind != "state" && kind != "sram" {
		return "", errors.New("invalid game save kind")
	}
	if slot < 0 || slot > 9 {
		return "", errors.New("invalid game save slot")
	}
	var platform, hash string
	if err := s.db.QueryRowContext(ctx, `SELECT platform,content_hash FROM games WHERE id=?`, gameID).Scan(&platform, &hash); err != nil {
		return "", err
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	hash = strings.ToLower(strings.TrimSpace(hash))
	if coreByPlatform[platform] == "" || !contentHashRE.MatchString(hash) {
		return "", errors.New("invalid persisted game identity")
	}
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir, "game-saves", fmt.Sprintf("profile-%d", profileID), platform, hash)
	name := fmt.Sprintf("state-%d.state", slot)
	if kind == "sram" {
		name = fmt.Sprintf("battery-%d.srm", slot)
	}
	return filepath.Join(dir, name), nil
}

func MaxSaveBytes(kind string) int64 {
	if kind == "sram" {
		return maxSRAMBytes
	}
	if kind == "state" {
		return maxStateBytes
	}
	return 0
}

func (s *Service) saveInfo(ctx context.Context, profileID, gameID int64, kind string, slot int) (SaveFile, error) {
	path, err := s.savePath(ctx, profileID, gameID, kind, slot)
	if err != nil {
		return SaveFile{}, err
	}
	gameSaveMu.Lock()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		previous := path + ".previous"
		if _, previousErr := os.Stat(previous); previousErr == nil {
			_ = os.Rename(previous, path)
		}
	}
	gameSaveMu.Unlock()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return SaveFile{Path: path, SaveInfo: SaveInfo{Kind: kind}}, nil
	}
	if err != nil {
		return SaveFile{}, err
	}
	var version int64
	_ = s.db.QueryRowContext(ctx, `SELECT version FROM game_saves WHERE profile_id=? AND game_id=? AND kind=? AND slot=?`, profileID, gameID, kind, slot).Scan(&version)
	return SaveFile{Path: path, SaveInfo: SaveInfo{Kind: kind, Exists: true, SizeBytes: info.Size(), Version: version, UpdatedAt: info.ModTime().UTC().Format(time.RFC3339)}}, nil
}

func (s *Service) SaveStatus(ctx context.Context, profileID, gameID int64) (SaveStatus, error) {
	state, err := s.saveInfo(ctx, profileID, gameID, "state", 0)
	if err != nil {
		return SaveStatus{}, err
	}
	sram, err := s.saveInfo(ctx, profileID, gameID, "sram", 0)
	if err != nil {
		return SaveStatus{}, err
	}
	return SaveStatus{State: state.SaveInfo, SRAM: sram.SaveInfo}, nil
}

func (s *Service) SaveFile(ctx context.Context, profileID, gameID int64, kind string, slot int) (SaveFile, error) {
	file, err := s.saveInfo(ctx, profileID, gameID, kind, slot)
	if err != nil {
		return SaveFile{}, err
	}
	if !file.Exists {
		return SaveFile{}, sql.ErrNoRows
	}
	return file, nil
}

func (s *Service) WriteSave(ctx context.Context, profileID, gameID int64, kind string, slot int, payload []byte) (SaveInfo, error) {
	max := MaxSaveBytes(kind)
	if max == 0 {
		return SaveInfo{}, errors.New("invalid game save kind")
	}
	if len(payload) == 0 {
		return SaveInfo{}, errors.New("empty game save payload")
	}
	if int64(len(payload)) > max {
		return SaveInfo{}, errors.New("game save payload is too large")
	}
	path, err := s.savePath(ctx, profileID, gameID, kind, slot)
	if err != nil {
		return SaveInfo{}, err
	}
	gameSaveMu.Lock()
	defer gameSaveMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return SaveInfo{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stormflix-save-*")
	if err != nil {
		return SaveInfo{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return SaveInfo{}, err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return SaveInfo{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return SaveInfo{}, err
	}
	if err := tmp.Close(); err != nil {
		return SaveInfo{}, err
	}

	// Keep three recovery generations. The .previous staging name makes a
	// failed final rename recoverable instead of silently losing the last save.
	_ = os.Remove(path + ".bak3")
	if _, err := os.Stat(path + ".bak2"); err == nil {
		_ = os.Rename(path+".bak2", path+".bak3")
	}
	if _, err := os.Stat(path + ".bak1"); err == nil {
		_ = os.Rename(path+".bak1", path+".bak2")
	}
	previous := path + ".previous"
	_ = os.Remove(previous)
	hadCurrent := false
	if _, err := os.Stat(path); err == nil {
		hadCurrent = true
		if err := os.Rename(path, previous); err != nil {
			return SaveInfo{}, err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		if hadCurrent {
			_ = os.Rename(previous, path)
		}
		return SaveInfo{}, err
	}
	if hadCurrent {
		_ = os.Rename(previous, path+".bak1")
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO game_saves(profile_id,game_id,kind,slot,size_bytes,version)
VALUES(?,?,?,?,?,1)
ON CONFLICT(profile_id,game_id,kind,slot) DO UPDATE SET
 size_bytes=excluded.size_bytes,version=game_saves.version+1,updated_at=CURRENT_TIMESTAMP`,
		profileID, gameID, kind, slot, len(payload))
	if err != nil {
		return SaveInfo{}, err
	}
	var version int64
	_ = s.db.QueryRowContext(ctx, `SELECT version FROM game_saves WHERE profile_id=? AND game_id=? AND kind=? AND slot=?`, profileID, gameID, kind, slot).Scan(&version)
	return SaveInfo{Kind: kind, Exists: true, SizeBytes: int64(len(payload)), Version: version, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

// Heartbeat credits only bounded positive deltas from one opaque browser
// session. A suspended tab or forged huge jump can add at most 120 seconds in a
// single heartbeat, while normal 15-second updates remain exact.
func (s *Service) Heartbeat(ctx context.Context, profileID, gameID int64, sessionID string, elapsedSeconds int64) (int64, error) {
	sessionID = strings.TrimSpace(sessionID)
	if profileID <= 0 || gameID <= 0 || !playSessionID.MatchString(sessionID) {
		return 0, errors.New("invalid game play session")
	}
	if elapsedSeconds < 0 {
		elapsedSeconds = 0
	}
	if elapsedSeconds > 24*60*60 {
		elapsedSeconds = 24 * 60 * 60
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var oldProfile, oldGame, previous int64
	err = tx.QueryRowContext(ctx, `SELECT profile_id,game_id,client_seconds FROM game_play_sessions WHERE session_id=?`, sessionID).Scan(&oldProfile, &oldGame, &previous)
	delta := int64(0)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO game_play_sessions(session_id,profile_id,game_id,client_seconds,credited_seconds) VALUES(?,?,?,?,0)`, sessionID, profileID, gameID, elapsedSeconds)
	} else if err == nil {
		if oldProfile != profileID || oldGame != gameID {
			return 0, errors.New("game play session identity changed")
		}
		delta = elapsedSeconds - previous
		if delta < 0 {
			delta = 0
		}
		if delta > 120 {
			delta = 120
		}
		_, err = tx.ExecContext(ctx, `UPDATE game_play_sessions SET client_seconds=?,credited_seconds=credited_seconds+?,last_seen_at=CURRENT_TIMESTAMP WHERE session_id=?`, elapsedSeconds, delta, sessionID)
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO game_profile_state(profile_id,game_id,play_seconds,last_played_at)
VALUES(?,?,?,CURRENT_TIMESTAMP)
ON CONFLICT(profile_id,game_id) DO UPDATE SET
 play_seconds=game_profile_state.play_seconds+excluded.play_seconds,last_played_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`,
		profileID, gameID, delta)
	if err != nil {
		return 0, err
	}
	var total int64
	if err := tx.QueryRowContext(ctx, `SELECT play_seconds FROM game_profile_state WHERE profile_id=? AND game_id=?`, profileID, gameID).Scan(&total); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM game_play_sessions WHERE last_seen_at<datetime('now','-2 days')`)
	return total, nil
}

func (s *Service) RuntimeAsset(ctx context.Context, asset string) (string, string, error) {
	asset = strings.TrimSpace(asset)
	var remoteURL, relative, contentType string
	if asset == "nostalgist.js" {
		remoteURL = "https://cdn.jsdelivr.net/npm/nostalgist@" + NostalgistVersion + "/dist/nostalgist.umd.js"
		relative = "nostalgist-" + NostalgistVersion + ".umd.js"
		contentType = "text/javascript; charset=utf-8"
	} else {
		ext := filepath.Ext(asset)
		core := strings.TrimSuffix(asset, ext)
		if !allowedRuntimeCores[core] || (ext != ".js" && ext != ".wasm") {
			return "", "", errors.New("unsupported game runtime asset")
		}
		remoteURL = fmt.Sprintf("https://cdn.jsdelivr.net/gh/arianrhodsandlot/retroarch-emscripten-build@%s/retroarch/%s_libretro%s", RetroArchBuild, core, ext)
		relative = filepath.Join("retroarch-"+RetroArchBuild, core+"_libretro"+ext)
		if ext == ".wasm" {
			contentType = "application/wasm"
		} else {
			contentType = "text/javascript; charset=utf-8"
		}
	}
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return "", "", err
	}
	destination := filepath.Join(dataDir, "game-runtime", relative)
	gameRuntimeMu.Lock()
	defer gameRuntimeMu.Unlock()
	if info, err := os.Stat(destination); err == nil && !info.IsDir() && info.Size() > 0 {
		return destination, contentType, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("User-Agent", "StormFlix-Games/1.0")
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", "", fmt.Errorf("download pinned game runtime: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("download pinned game runtime: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxRuntimeBytes {
		return "", "", errors.New("pinned game runtime asset exceeds safety limit")
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".stormflix-runtime-*")
	if err != nil {
		return "", "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	n, copyErr := io.Copy(tmp, io.LimitReader(response.Body, maxRuntimeBytes+1))
	if copyErr == nil && n > maxRuntimeBytes {
		copyErr = errors.New("pinned game runtime asset exceeds safety limit")
	}
	if copyErr == nil && n == 0 {
		copyErr = errors.New("pinned game runtime asset is empty")
	}
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if copyErr != nil {
		return "", "", copyErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return "", "", err
	}
	return destination, contentType, nil
}
