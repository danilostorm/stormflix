package games

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var gameProviderMu sync.Mutex

type ProviderField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
}

type ProviderInfo struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Kind        string            `json:"kind"`
	Stage       string            `json:"stage"`
	Enabled     bool              `json:"enabled"`
	Configured  bool              `json:"configured"`
	Public      map[string]string `json:"public"`
	Secrets     map[string]bool   `json:"secrets"`
	Fields      []ProviderField   `json:"fields"`
}

type ProviderUpdate struct {
	Provider string            `json:"provider"`
	Enabled  bool              `json:"enabled"`
	Values   map[string]string `json:"values"`
}

type AdminOverview struct {
	Libraries         int64          `json:"libraries"`
	Games             int64          `json:"games"`
	Files             int64          `json:"files"`
	Saves             int64          `json:"saves"`
	ProfilesWithSaves int64          `json:"profiles_with_saves"`
	ActivePlayers     int64          `json:"active_players"`
	PlaySeconds       int64          `json:"play_seconds"`
	MetadataRows      int64          `json:"metadata_rows"`
	LockedMetadata    int64          `json:"locked_metadata"`
	Platforms         []PlatformStat `json:"platforms"`
	RecentScans       []Job          `json:"recent_scans"`
}

type AdminGame struct {
	ID             int64  `json:"id"`
	LibraryID      int64  `json:"library_id"`
	Library        string `json:"library"`
	Platform       string `json:"platform"`
	Title          string `json:"title"`
	ContentHash    string `json:"content_hash"`
	FileCount      int    `json:"file_count"`
	AvailableFiles int    `json:"available_files"`
	SaveCount      int    `json:"save_count"`
	PlaySeconds    int64  `json:"play_seconds"`
	Provider       string `json:"provider"`
	MetadataID     string `json:"metadata_id"`
	ReleaseYear    int    `json:"release_year"`
	MetadataLocked bool   `json:"metadata_locked"`
	UpdatedAt      string `json:"updated_at"`
}

type SaveGalleryItem struct {
	GameID       int64  `json:"game_id"`
	Title        string `json:"title"`
	Platform     string `json:"platform"`
	Library      string `json:"library"`
	CoverURL     string `json:"cover_url"`
	Kind         string `json:"kind"`
	Slot         int    `json:"slot"`
	SizeBytes    int64  `json:"size_bytes"`
	Version      int64  `json:"version"`
	UpdatedAt    string `json:"updated_at"`
	PlaySeconds  int64  `json:"play_seconds"`
	LastPlayedAt string `json:"last_played_at"`
}

type providerDefinition struct {
	Key         string
	Name        string
	Description string
	Kind        string
	Stage       string
	Fields      []ProviderField
}

var providerDefinitions = []providerDefinition{
	{Key: "igdb", Name: "IGDB", Kind: "principal", Stage: "configuravel", Description: "Título, resumo, data, gêneros, empresas, ratings e IDs.", Fields: []ProviderField{{Key: "client_id", Label: "Client ID", Required: true, Placeholder: "IGDB / Twitch Client ID"}, {Key: "client_secret", Label: "Client Secret", Secret: true, Required: true, Placeholder: "••••••••"}}},
	{Key: "steamgriddb", Name: "SteamGridDB", Kind: "artwork", Stage: "configuravel", Description: "Capas, heroes, logos e artwork de alta qualidade.", Fields: []ProviderField{{Key: "api_key", Label: "API Key", Secret: true, Required: true, Placeholder: "••••••••"}}},
	{Key: "mobygames", Name: "MobyGames", Kind: "principal", Stage: "configuravel", Description: "Metadados editoriais, plataformas, créditos e lançamentos.", Fields: []ProviderField{{Key: "api_key", Label: "API Key", Secret: true, Required: true, Placeholder: "••••••••"}}},
	{Key: "screenscraper", Name: "ScreenScraper", Kind: "principal", Stage: "configuravel", Description: "Hashes, mídia, regiões, nomes e metadados de ampla cobertura retro.", Fields: []ProviderField{{Key: "username", Label: "Usuário", Required: true, Placeholder: "ScreenScraper"}, {Key: "password", Label: "Senha", Secret: true, Required: true, Placeholder: "••••••••"}}},
	{Key: "retroachievements", Name: "RetroAchievements", Kind: "enriquecimento", Stage: "configuravel", Description: "Identidade por hash, conquistas e dados de comunidade.", Fields: []ProviderField{{Key: "api_key", Label: "API Key", Secret: true, Required: true, Placeholder: "••••••••"}}},
	{Key: "hasheous", Name: "Hasheous", Kind: "hash", Stage: "planejado", Description: "Correspondência por hash e IDs cruzados para reduzir falsos positivos."},
	{Key: "playmatch", Name: "PlayMatch", Kind: "hash", Stage: "planejado", Description: "Correspondência adicional de ROMs e plataformas."},
	{Key: "launchbox", Name: "LaunchBox", Kind: "offline", Stage: "planejado", Description: "Base local/baixável para nomes, imagens e dados de plataformas."},
	{Key: "thegamesdb", Name: "TheGamesDB", Kind: "principal", Stage: "planejado", Description: "Fonte complementar de metadados e artwork."},
	{Key: "flashpoint", Name: "Flashpoint", Kind: "especializado", Stage: "planejado", Description: "Metadados para jogos Flash e títulos de navegador preservados."},
	{Key: "hltb", Name: "HowLongToBeat", Kind: "enriquecimento", Stage: "planejado", Description: "Estimativas de duração para campanha e conclusão."},
	{Key: "demozoo", Name: "Demozoo", Kind: "especializado", Stage: "planejado", Description: "Metadados para demos e produções da demoscene."},
	{Key: "pouet", Name: "Pouët", Kind: "especializado", Stage: "planejado", Description: "Fonte complementar para produções da demoscene."},
	{Key: "csdb", Name: "CSDb", Kind: "especializado", Stage: "planejado", Description: "Metadados especializados para Commodore 64."},
	{Key: "libretro", Name: "Libretro", Kind: "artwork", Stage: "planejado", Description: "Thumbnails e identidade compatível com ecossistema RetroArch."},
}

func providerDefinitionFor(key string) (providerDefinition, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, definition := range providerDefinitions {
		if definition.Key == key {
			return definition, true
		}
	}
	return providerDefinition{}, false
}

func (s *Service) AdminOverview(ctx context.Context) (AdminOverview, error) {
	var out AdminOverview
	queries := []struct {
		query string
		dest  *int64
	}{
		{`SELECT COUNT(*) FROM libraries WHERE lower(trim(kind))='games'`, &out.Libraries},
		{`SELECT COUNT(*) FROM games`, &out.Games},
		{`SELECT COUNT(*) FROM game_files WHERE available=1`, &out.Files},
		{`SELECT COUNT(*) FROM game_saves`, &out.Saves},
		{`SELECT COUNT(DISTINCT profile_id) FROM game_saves`, &out.ProfilesWithSaves},
		{`SELECT COUNT(*) FROM game_play_sessions WHERE last_seen_at>=datetime('now','-90 seconds')`, &out.ActivePlayers},
		{`SELECT COALESCE(SUM(play_seconds),0) FROM game_profile_state`, &out.PlaySeconds},
		{`SELECT COUNT(*) FROM game_metadata`, &out.MetadataRows},
		{`SELECT COUNT(*) FROM game_metadata WHERE metadata_locked=1`, &out.LockedMetadata},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query).Scan(item.dest); err != nil {
			return AdminOverview{}, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT platform,COUNT(*) FROM games GROUP BY platform ORDER BY COUNT(*) DESC,platform`)
	if err != nil {
		return AdminOverview{}, err
	}
	for rows.Next() {
		var stat PlatformStat
		if err := rows.Scan(&stat.Platform, &stat.Count); err != nil {
			_ = rows.Close()
			return AdminOverview{}, err
		}
		stat.Label = PlatformLabel(stat.Platform)
		out.Platforms = append(out.Platforms, stat)
	}
	_ = rows.Close()
	out.RecentScans, _ = s.Jobs(ctx, 8)
	return out, nil
}

func (s *Service) AdminCatalog(ctx context.Context, query, platform string, limit int) ([]AdminGame, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	where := ` WHERE 1=1`
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		where += ` AND lower(g.title) LIKE ?`
		args = append(args, "%"+strings.ToLower(q)+"%")
	}
	if p := strings.ToLower(strings.TrimSpace(platform)); p != "" {
		where += ` AND g.platform=?`
		args = append(args, p)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT g.id,g.library_id,l.name,g.platform,g.title,g.content_hash,
       COUNT(DISTINCT gf.id),SUM(CASE WHEN gf.available=1 THEN 1 ELSE 0 END),
       COUNT(DISTINCT gs.profile_id||':'||gs.kind||':'||gs.slot),
       COALESCE(SUM(DISTINCT gps.play_seconds),0),
       COALESCE(NULLIF(g.metadata_provider,''),gm.primary_provider,''),
       COALESCE(NULLIF(g.metadata_id,''),gm.primary_id,''),g.release_year,
       COALESCE(gm.metadata_locked,0),g.updated_at
FROM games g
JOIN libraries l ON l.id=g.library_id
LEFT JOIN game_files gf ON gf.game_id=g.id
LEFT JOIN game_saves gs ON gs.game_id=g.id
LEFT JOIN game_profile_state gps ON gps.game_id=g.id
LEFT JOIN game_metadata gm ON gm.game_id=g.id`+where+`
GROUP BY g.id
ORDER BY g.sort_title,g.title,g.id
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminGame{}
	for rows.Next() {
		var item AdminGame
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.Library, &item.Platform, &item.Title, &item.ContentHash, &item.FileCount, &item.AvailableFiles, &item.SaveCount, &item.PlaySeconds, &item.Provider, &item.MetadataID, &item.ReleaseYear, &item.MetadataLocked, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveGallery(ctx context.Context, profileID int64, allowed []int64, limit int) ([]SaveGalleryItem, error) {
	if profileID <= 0 {
		return []SaveGalleryItem{}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	where, accessArgs := allowedClause("g.library_id", allowed)
	args := []any{profileID, profileID}
	args = append(args, accessArgs...)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT gs.game_id,g.title,g.platform,l.name,g.cover_path,gs.kind,gs.slot,gs.size_bytes,gs.version,gs.updated_at,
       COALESCE(ps.play_seconds,0),COALESCE(ps.last_played_at,'')
FROM game_saves gs
JOIN games g ON g.id=gs.game_id
JOIN libraries l ON l.id=g.library_id
LEFT JOIN game_profile_state ps ON ps.game_id=g.id AND ps.profile_id=?
WHERE gs.profile_id=?`+where+`
ORDER BY gs.updated_at DESC,gs.game_id,gs.kind,gs.slot
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SaveGalleryItem{}
	for rows.Next() {
		var item SaveGalleryItem
		var cover string
		if err := rows.Scan(&item.GameID, &item.Title, &item.Platform, &item.Library, &cover, &item.Kind, &item.Slot, &item.SizeBytes, &item.Version, &item.UpdatedAt, &item.PlaySeconds, &item.LastPlayedAt); err != nil {
			return nil, err
		}
		if strings.TrimSpace(cover) != "" {
			item.CoverURL = fmt.Sprintf("/api/v1/games/%d/cover", item.GameID)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) providerKey(ctx context.Context) ([]byte, error) {
	dataDir, err := s.dataDir(ctx)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "game-providers.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid games provider encryption key")
	}
	return key, nil
}

func encryptProviderSecrets(key []byte, values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	plain, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptProviderSecrets(key []byte, value string) (map[string]string, error) {
	out := map[string]string{}
	value = strings.TrimSpace(value)
	if value == "" {
		return out, nil
	}
	payload, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted games provider payload")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) providerStored(ctx context.Context, provider string) (bool, map[string]string, map[string]string, error) {
	var enabled bool
	var publicJSON, encrypted string
	err := s.db.QueryRowContext(ctx, `SELECT enabled,public_json,secret_ciphertext FROM game_provider_settings WHERE provider=?`, provider).Scan(&enabled, &publicJSON, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, map[string]string{}, map[string]string{}, nil
	}
	if err != nil {
		return false, nil, nil, err
	}
	publicValues := map[string]string{}
	_ = json.Unmarshal([]byte(publicJSON), &publicValues)
	key, err := s.providerKey(ctx)
	if err != nil {
		return false, nil, nil, err
	}
	secrets, err := decryptProviderSecrets(key, encrypted)
	return enabled, publicValues, secrets, err
}

func (s *Service) ProviderSettings(ctx context.Context) ([]ProviderInfo, error) {
	out := make([]ProviderInfo, 0, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		enabled, publicValues, secretValues, err := s.providerStored(ctx, definition.Key)
		if err != nil {
			return nil, err
		}
		info := ProviderInfo{Key: definition.Key, Name: definition.Name, Description: definition.Description, Kind: definition.Kind, Stage: definition.Stage, Enabled: enabled, Public: publicValues, Secrets: map[string]bool{}, Fields: definition.Fields}
		configured := true
		for _, field := range definition.Fields {
			value := strings.TrimSpace(publicValues[field.Key])
			if field.Secret {
				value = strings.TrimSpace(secretValues[field.Key])
				info.Secrets[field.Key] = value != ""
			}
			if field.Required && value == "" {
				configured = false
			}
		}
		if len(definition.Fields) == 0 {
			configured = true
		}
		info.Configured = configured
		out = append(out, info)
	}
	return out, nil
}

func (s *Service) UpdateProviderSettings(ctx context.Context, in ProviderUpdate) error {
	definition, ok := providerDefinitionFor(in.Provider)
	if !ok {
		return errors.New("unknown games metadata provider")
	}
	gameProviderMu.Lock()
	defer gameProviderMu.Unlock()

	_, publicValues, secretValues, err := s.providerStored(ctx, definition.Key)
	if err != nil {
		return err
	}
	allowed := map[string]ProviderField{}
	for _, field := range definition.Fields {
		allowed[field.Key] = field
	}
	for key, raw := range in.Values {
		field, exists := allowed[key]
		if !exists {
			continue
		}
		value := strings.TrimSpace(raw)
		if field.Secret {
			if value == "" {
				continue
			}
			if value == "__clear__" {
				delete(secretValues, key)
			} else {
				secretValues[key] = value
			}
		} else {
			if value == "__clear__" {
				value = ""
			}
			publicValues[key] = value
		}
	}
	publicJSON, err := json.Marshal(publicValues)
	if err != nil {
		return err
	}
	key, err := s.providerKey(ctx)
	if err != nil {
		return err
	}
	encrypted, err := encryptProviderSecrets(key, secretValues)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO game_provider_settings(provider,enabled,public_json,secret_ciphertext)
VALUES(?,?,?,?)
ON CONFLICT(provider) DO UPDATE SET enabled=excluded.enabled,public_json=excluded.public_json,secret_ciphertext=excluded.secret_ciphertext,updated_at=CURRENT_TIMESTAMP`, definition.Key, in.Enabled, string(publicJSON), encrypted)
	return err
}

func (s *Service) ProviderSecretsForRuntime(ctx context.Context, provider string) (map[string]string, map[string]string, bool, error) {
	definition, ok := providerDefinitionFor(provider)
	if !ok {
		return nil, nil, false, errors.New("unknown games metadata provider")
	}
	enabled, publicValues, secretValues, err := s.providerStored(ctx, definition.Key)
	return publicValues, secretValues, enabled, err
}

func ProviderKeys() []string {
	keys := make([]string, 0, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		keys = append(keys, definition.Key)
	}
	sort.Strings(keys)
	return keys
}
