package games

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type hasheousBridge struct {
	ID       string
	Name     string
	CoverURL string
	IDs      map[string]string
}

var screenScraperPlatformIDs = map[string]int{
	"nes": 3, "snes": 4, "genesis": 1, "gb": 9, "gbc": 10, "gba": 12,
}

func (s *Service) enrichGameStackV2(ctx context.Context, game metadataGameRow) (string, *gameMetadataCandidate, error) {
	providerIDs := map[string]string{}
	var errs []string

	hashes, hashErr := s.gameLookupHashes(ctx, game.ID)
	if hashErr != nil {
		errs = append(errs, "hashes: "+hashErr.Error())
	}

	// 1. Hash identity bridge. Hasheous is intentionally not allowed to replace
	// the local platform+SHA-256 identity; it only contributes correlated IDs.
	var bridge *hasheousBridge
	if public, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "hasheous"); enabled && hashErr == nil {
		baseURL := strings.TrimSpace(public["base_url"])
		if baseURL == "" {
			baseURL = "https://hasheous.org/api/v1"
		}
		if apiKey := strings.TrimSpace(secrets["api_key"]); apiKey != "" {
			value, err := fetchHasheous(ctx, baseURL, apiKey, hashes)
			if err != nil {
				errs = append(errs, "Hasheous: "+err.Error())
			} else if value != nil {
				bridge = value
				providerIDs["hasheous"] = value.ID
				for key, id := range value.IDs {
					if strings.TrimSpace(id) != "" {
						providerIDs[key] = strings.TrimSpace(id)
					}
				}
			}
		}
	}

	// 2. Primary metadata sources. ScreenScraper gets first chance because it can
	// use ROM hashes directly. Title-oriented sources are conservative fallbacks.
	var candidate *gameMetadataCandidate
	if public, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "screenscraper"); enabled && hashErr == nil {
		devID := strings.TrimSpace(public["dev_id"])
		devPassword := strings.TrimSpace(secrets["dev_password"])
		username := strings.TrimSpace(public["username"])
		password := strings.TrimSpace(secrets["password"])
		if devID != "" && devPassword != "" && username != "" && password != "" {
			value, err := fetchScreenScraper(ctx, devID, devPassword, username, password, game, hashes)
			if err != nil {
				errs = append(errs, "ScreenScraper: "+err.Error())
			} else if value != nil {
				candidate = value
				providerIDs["screenscraper"] = value.ProviderID
			}
		}
	}

	if candidate == nil {
		if public, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "igdb"); enabled {
			clientID := strings.TrimSpace(public["client_id"])
			clientSecret := strings.TrimSpace(secrets["client_secret"])
			if clientID != "" && clientSecret != "" {
				value, err := fetchIGDB(ctx, clientID, clientSecret, game)
				if err != nil {
					errs = append(errs, "IGDB: "+err.Error())
				} else if value != nil {
					candidate = value
					providerIDs["igdb"] = value.ProviderID
				}
			}
		}
	}

	if candidate == nil {
		if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "mobygames"); enabled {
			if apiKey := strings.TrimSpace(secrets["api_key"]); apiKey != "" {
				value, err := fetchMobyGames(ctx, apiKey, game)
				if err != nil {
					errs = append(errs, "MobyGames: "+err.Error())
				} else if value != nil {
					candidate = value
					providerIDs["mobygames"] = value.ProviderID
				}
			}
		}
	}

	if candidate == nil {
		if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "thegamesdb"); enabled {
			if apiKey := strings.TrimSpace(secrets["api_key"]); apiKey != "" {
				value, err := fetchTheGamesDB(ctx, apiKey, game)
				if err != nil {
					errs = append(errs, "TheGamesDB: "+err.Error())
				} else if value != nil {
					candidate = value
					providerIDs["thegamesdb"] = value.ProviderID
				}
			}
		}
	}

	if candidate == nil && bridge != nil && strings.TrimSpace(bridge.Name) != "" && titleScore(game.Title, bridge.Name) >= .55 {
		candidate = &gameMetadataCandidate{
			Provider:   "hasheous",
			ProviderID: bridge.ID,
			Title:      bridge.Name,
			CoverURL:   bridge.CoverURL,
		}
	}

	// Persist hash-correlated IDs even when no primary metadata source matched.
	if err := s.persistProviderIDs(ctx, game.ID, providerIDs); err != nil {
		errs = append(errs, "provider ids: "+err.Error())
	}
	if candidate == nil {
		if len(errs) > 0 {
			return "", nil, errors.New(strings.Join(errs, " · "))
		}
		return "", nil, nil
	}

	providerIDs[candidate.Provider] = candidate.ProviderID

	// 3. RetroAchievements enriches a *verified* game ID. We intentionally do
	// not treat a normal file MD5/SHA1 as an RA hash because RA hashing is
	// platform-specific for several systems.
	if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "retroachievements"); enabled {
		apiKey := strings.TrimSpace(secrets["api_key"])
		if apiKey != "" {
			raID := strings.TrimSpace(providerIDs["retroachievements"])
			if raID == "" {
				raID = explicitRAID(game.Title)
			}
			if raID != "" {
				ra, err := fetchRetroAchievements(ctx, apiKey, raID)
				if err != nil {
					errs = append(errs, "RetroAchievements: "+err.Error())
				} else if ra != nil {
					providerIDs["retroachievements"] = ra.ProviderID
					mergeMetadataCandidate(candidate, ra)
				}
			}
		}
	}

	// 4. Artwork enrichment. SteamGridDB is preferred for polished portrait art;
	// Libretro is a public, no-secret fallback aligned with the RetroArch stack.
	if _, secrets, enabled, _ := s.ProviderSecretsForRuntime(ctx, "steamgriddb"); enabled {
		if apiKey := strings.TrimSpace(secrets["api_key"]); apiKey != "" {
			id, cover, err := fetchSteamGridDB(ctx, apiKey, candidate.Title)
			if err != nil {
				errs = append(errs, "SteamGridDB: "+err.Error())
			} else if id > 0 {
				candidate.SteamGridDBID = strconv.FormatInt(id, 10)
				providerIDs["steamgriddb"] = candidate.SteamGridDBID
				if cover != "" {
					candidate.CoverURL = cover
				}
			}
		}
	}

	if candidate.CoverURL == "" {
		if _, _, enabled, _ := s.ProviderSecretsForRuntime(ctx, "libretro"); enabled {
			if cover := fetchLibretroThumbnail(ctx, game.Platform, candidate.Title); cover != "" {
				candidate.CoverURL = cover
				providerIDs["libretro"] = normalizeGameTitle(candidate.Title)
			}
		}
	}

	if err := s.persistProviderIDs(ctx, game.ID, providerIDs); err != nil {
		errs = append(errs, "provider ids: "+err.Error())
	}
	return candidate.Provider, candidate, nil
}

func mergeMetadataCandidate(dst, src *gameMetadataCandidate) {
	if dst == nil || src == nil {
		return
	}
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.Overview == "" {
		dst.Overview = src.Overview
	}
	if dst.ReleaseYear == 0 {
		dst.ReleaseYear = src.ReleaseYear
	}
	if dst.Rating == 0 {
		dst.Rating = src.Rating
	}
	if len(dst.Genres) == 0 {
		dst.Genres = append([]string(nil), src.Genres...)
	}
	if len(dst.Developers) == 0 {
		dst.Developers = append([]string(nil), src.Developers...)
	}
	if len(dst.Publishers) == 0 {
		dst.Publishers = append([]string(nil), src.Publishers...)
	}
	if dst.CoverURL == "" {
		dst.CoverURL = src.CoverURL
	}
	for _, shot := range src.Screenshots {
		if shot != "" && !containsString(dst.Screenshots, shot) {
			dst.Screenshots = append(dst.Screenshots, shot)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func explicitRAID(title string) string {
	lower := strings.ToLower(title)
	start := strings.Index(lower, "(ra-")
	if start < 0 {
		return ""
	}
	start += 4
	end := strings.IndexByte(lower[start:], ')')
	if end < 0 {
		return ""
	}
	value := lower[start : start+end]
	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		return ""
	}
	return value
}

func fetchHasheous(ctx context.Context, baseURL, apiKey string, hashes gameLookupHashSet) (*hasheousBridge, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsedBase, err := url.Parse(baseURL)
	if err != nil || (parsedBase.Scheme != "https" && parsedBase.Scheme != "http") || parsedBase.Host == "" {
		return nil, errors.New("invalid Hasheous base URL")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, nil
	}
	payload := []map[string]string{{"mD5": hashes.MD5, "shA1": hashes.SHA1, "crc": hashes.CRC}}
	body, _ := json.Marshal(payload)
	endpoint := baseURL + "/Lookup/ByHash?returnAllSources=true&returnFields=Signatures%2CMetadata%2CAttributes"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json-patch+json")
	req.Header.Set("X-Client-API-Key", apiKey)
	req.Header.Set("User-Agent", "StormFlix/MetadataStack-v2")
	resp, err := (&http.Client{Timeout: 35 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		ID       any    `json:"id"`
		Name     string `json:"name"`
		Metadata []struct {
			Source      string `json:"source"`
			ImmutableID any    `json:"immutableId"`
		} `json:"metadata"`
		Attributes []struct {
			Name string `json:"attributeName"`
			Link string `json:"link"`
		} `json:"attributes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 3<<20)).Decode(&out); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(fmt.Sprint(out.ID))
	if id == "" || id == "<nil>" {
		return nil, nil
	}
	bridge := &hasheousBridge{ID: id, Name: strings.TrimSpace(out.Name), IDs: map[string]string{}}
	for _, meta := range out.Metadata {
		value := strings.TrimSpace(fmt.Sprint(meta.ImmutableID))
		if value == "" || value == "<nil>" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(meta.Source)) {
		case "igdb":
			if _, err := strconv.ParseInt(value, 10, 64); err == nil {
				bridge.IDs["igdb"] = value
			}
		case "retroachievements":
			bridge.IDs["retroachievements"] = value
		case "thegamesdb":
			bridge.IDs["thegamesdb"] = value
		}
	}
	origin := parsedBase.Scheme + "://" + parsedBase.Host
	for _, attr := range out.Attributes {
		if strings.EqualFold(attr.Name, "Logo") && strings.TrimSpace(attr.Link) != "" {
			if strings.HasPrefix(attr.Link, "http://") || strings.HasPrefix(attr.Link, "https://") {
				bridge.CoverURL = attr.Link
			} else {
				bridge.CoverURL = origin + "/" + strings.TrimLeft(attr.Link, "/")
			}
			break
		}
	}
	return bridge, nil
}

func fetchScreenScraper(ctx context.Context, devID, devPassword, username, password string, game metadataGameRow, hashes gameLookupHashSet) (*gameMetadataCandidate, error) {
	systemID := screenScraperPlatformIDs[game.Platform]
	if systemID == 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("devid", devID)
	q.Set("devpassword", devPassword)
	q.Set("softname", "StormFlix")
	q.Set("ssid", username)
	q.Set("sspassword", password)
	q.Set("output", "json")
	q.Set("systemeid", strconv.Itoa(systemID))
	q.Set("romnom", game.Title)
	q.Set("rommd5", hashes.MD5)
	q.Set("romsha1", hashes.SHA1)
	q.Set("romcrc", hashes.CRC)

	endpoint := "https://www.screenscraper.fr/api2/jeuInfos.php?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "StormFlix/MetadataStack-v2")
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var root map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 6<<20)).Decode(&root); err != nil {
		return nil, err
	}
	response := mapValue(root["response"])
	jeu := mapValue(response["jeu"])
	if len(jeu) == 0 {
		return nil, nil
	}
	id := stringValue(jeu["id"])
	name := localizedText(jeu["noms"], "region", []string{"us", "wor", "eu", "ss", "jp"})
	if name == "" {
		name = game.Title
	}
	out := &gameMetadataCandidate{Provider: "screenscraper", ProviderID: id, Title: name}
	out.Overview = localizedText(jeu["synopsis"], "langue", []string{"en", "us", "fr"})
	if date := localizedText(jeu["dates"], "region", []string{"us", "wor", "eu", "jp"}); len(date) >= 4 {
		out.ReleaseYear, _ = strconv.Atoi(date[:4])
	}
	if note := stringValue(jeu["note"]); note != "" {
		if n, err := strconv.ParseFloat(note, 64); err == nil {
			if n > 10 {
				n /= 2
			}
			out.Rating = n
		}
	}
	if dev := nestedText(jeu["developpeur"]); dev != "" {
		out.Developers = []string{dev}
	}
	if pub := nestedText(jeu["editeur"]); pub != "" {
		out.Publishers = []string{pub}
	}
	for _, genre := range sliceValue(jeu["genres"]) {
		g := mapValue(genre)
		if label := localizedText(g["noms"], "langue", []string{"en", "fr"}); label != "" {
			out.Genres = append(out.Genres, label)
		}
	}
	for _, entry := range sliceValue(jeu["medias"]) {
		m := mapValue(entry)
		kind := strings.ToLower(stringValue(m["type"]))
		raw := stringValue(m["url"])
		if raw == "" {
			continue
		}
		switch kind {
		case "box-2d", "box2d", "box-2d-side", "box-texture":
			if out.CoverURL == "" {
				out.CoverURL = raw
			}
		case "ss", "sstitle", "screenshot":
			out.Screenshots = append(out.Screenshots, raw)
		}
	}
	return out, nil
}

func mapValue(v any) map[string]any {
	if value, ok := v.(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func sliceValue(v any) []any {
	if value, ok := v.([]any); ok {
		return value
	}
	return nil
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	case json.Number:
		return value.String()
	default:
		if v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
}

func nestedText(v any) string {
	m := mapValue(v)
	if text := stringValue(m["text"]); text != "" {
		return text
	}
	return stringValue(m["nom"])
}

func localizedText(v any, key string, preferred []string) string {
	items := sliceValue(v)
	for _, want := range preferred {
		for _, entry := range items {
			m := mapValue(entry)
			if strings.EqualFold(stringValue(m[key]), want) {
				if text := stringValue(m["text"]); text != "" {
					return text
				}
			}
		}
	}
	for _, entry := range items {
		if text := stringValue(mapValue(entry)["text"]); text != "" {
			return text
		}
	}
	return ""
}

func fetchRetroAchievements(ctx context.Context, apiKey, raID string) (*gameMetadataCandidate, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raID), 10, 64)
	if err != nil || id <= 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("i", strconv.FormatInt(id, 10))
	q.Set("y", apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://retroachievements.org/API/API_GetGameExtended.php?"+q.Encode(), nil)
	req.Header.Set("User-Agent", "StormFlix/MetadataStack-v2")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var item struct {
		ID          int64  `json:"ID"`
		Title       string `json:"Title"`
		Released    string `json:"Released"`
		Genre       string `json:"Genre"`
		Developer   string `json:"Developer"`
		Publisher   string `json:"Publisher"`
		ImageTitle  string `json:"ImageTitle"`
		ImageIngame string `json:"ImageIngame"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 3<<20)).Decode(&item); err != nil {
		return nil, err
	}
	if item.ID <= 0 {
		return nil, nil
	}
	out := &gameMetadataCandidate{Provider: "retroachievements", ProviderID: strconv.FormatInt(item.ID, 10), Title: item.Title}
	if len(item.Released) >= 4 {
		out.ReleaseYear, _ = strconv.Atoi(item.Released[:4])
	}
	if item.Genre != "" {
		out.Genres = []string{item.Genre}
	}
	if item.Developer != "" {
		out.Developers = []string{item.Developer}
	}
	if item.Publisher != "" {
		out.Publishers = []string{item.Publisher}
	}
	if item.ImageTitle != "" {
		out.CoverURL = absoluteRAMedia(item.ImageTitle)
	}
	if item.ImageIngame != "" {
		out.Screenshots = []string{absoluteRAMedia(item.ImageIngame)}
	}
	return out, nil
}

func absoluteRAMedia(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return "https://media.retroachievements.org" + raw
}

func fetchTheGamesDB(ctx context.Context, apiKey string, game metadataGameRow) (*gameMetadataCandidate, error) {
	q := url.Values{}
	q.Set("apikey", apiKey)
	q.Set("name", game.Title)
	q.Set("include", "boxart")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.thegamesdb.net/v1/Games/ByGameName?"+q.Encode(), nil)
	req.Header.Set("User-Agent", "StormFlix/MetadataStack-v2")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var root map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&root); err != nil {
		return nil, err
	}
	data := mapValue(root["data"])
	games := sliceValue(data["games"])
	best := map[string]any{}
	bestScore := 0.0
	for _, raw := range games {
		item := mapValue(raw)
		score := titleScore(game.Title, stringValue(item["game_title"]))
		if score > bestScore {
			bestScore = score
			best = item
		}
	}
	if bestScore < .66 || len(best) == 0 {
		return nil, nil
	}
	id := stringValue(best["id"])
	out := &gameMetadataCandidate{Provider: "thegamesdb", ProviderID: id, Title: stringValue(best["game_title"]), Overview: stringValue(best["overview"])}
	if date := stringValue(best["release_date"]); len(date) >= 4 {
		out.ReleaseYear, _ = strconv.Atoi(date[:4])
	}
	include := mapValue(root["include"])
	boxart := mapValue(include["boxart"])
	base := mapValue(boxart["base_url"])
	original := stringValue(base["original"])
	dataMap := mapValue(boxart["data"])
	for _, raw := range sliceValue(dataMap[id]) {
		art := mapValue(raw)
		side := strings.ToLower(stringValue(art["side"]))
		file := stringValue(art["filename"])
		if (side == "front" || side == "") && file != "" {
			out.CoverURL = strings.TrimRight(original, "/") + "/" + strings.TrimLeft(file, "/")
			break
		}
	}
	return out, nil
}

var libretroSystems = map[string]string{
	"nes":     "Nintendo - Nintendo Entertainment System",
	"snes":    "Nintendo - Super Nintendo Entertainment System",
	"genesis": "Sega - Mega Drive - Genesis",
	"gb":      "Nintendo - Game Boy",
	"gbc":     "Nintendo - Game Boy Color",
	"gba":     "Nintendo - Game Boy Advance",
}

func fetchLibretroThumbnail(ctx context.Context, platform, title string) string {
	system := libretroSystems[platform]
	if system == "" || strings.TrimSpace(title) == "" {
		return ""
	}
	name := strings.NewReplacer(
		"&", "_", "/", "_", "\\", "_", "?", "", "*", "", "\"", "", "<", "", ">", "", "|", "",
	).Replace(strings.TrimSpace(title)) + ".png"
	u := "https://thumbnails.libretro.com/" + url.PathEscape(system) + "/Named_Boxarts/" + url.PathEscape(name)
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return u
	}
	return ""
}
