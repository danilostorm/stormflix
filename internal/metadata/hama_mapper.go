package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HAMAMapper implements the useful part of the classic HAMA approach for a
// native StormFlix server: bridge anime IDs into TVDB/TMDB using the community
// Anime-Lists mapping data. It does not embed or execute the Plex HAMA plugin.
type HAMAMapper struct {
	client *http.Client
	mu sync.RWMutex
	loadedAt time.Time
	byAniList map[int64]hamaMapEntry
	byMAL map[int64]hamaMapEntry
	byAniDB map[int64]hamaMapEntry
}

type hamaMapEntry struct {
	Type string `json:"type"`
	AniDBID int64 `json:"anidb_id"`
	AniListID int64 `json:"anilist_id"`
	MALID int64 `json:"mal_id"`
	TVDBID int64 `json:"tvdb_id"`
	TheMovieDB struct {
		TV int64 `json:"tv"`
		Movie []int64 `json:"movie"`
	} `json:"themoviedb_id"`
	Season struct {
		TVDB int `json:"tvdb"`
		TMDB int `json:"tmdb"`
	} `json:"season"`
	EpisodeOffset struct {
		TVDB int `json:"tvdb"`
		TMDB int `json:"tmdb"`
	} `json:"episode_offset"`
}

func NewHAMAMapper() *HAMAMapper {
	return &HAMAMapper{
		client: &http.Client{Timeout: 25 * time.Second},
		byAniList: map[int64]hamaMapEntry{},
		byMAL: map[int64]hamaMapEntry{},
		byAniDB: map[int64]hamaMapEntry{},
	}
}

func (m *HAMAMapper) Ready() bool { return m != nil }

func (m *HAMAMapper) Enrich(ctx context.Context, result *Result) error {
	if m == nil || result == nil {
		return errors.New("HAMA mapping unavailable")
	}
	if err := m.ensureLoaded(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var entry hamaMapEntry
	var ok bool
	if result.AniListID > 0 {
		entry, ok = m.byAniList[result.AniListID]
	}
	if !ok && result.MALID > 0 {
		entry, ok = m.byMAL[result.MALID]
	}
	if !ok {
		return errors.New("HAMA mapping: no bridge for anime IDs")
	}
	if result.TVDBID == 0 && entry.TVDBID > 0 {
		result.TVDBID = entry.TVDBID
	}
	if result.TMDBID == 0 {
		if entry.TheMovieDB.TV > 0 {
			result.TMDBID = entry.TheMovieDB.TV
		} else if len(entry.TheMovieDB.Movie) > 0 {
			result.TMDBID = entry.TheMovieDB.Movie[0]
		}
	}
	if result.Season <= 0 {
		if entry.Season.TMDB > 0 {
			result.Season = entry.Season.TMDB
		} else if entry.Season.TVDB > 0 {
			result.Season = entry.Season.TVDB
		}
	}
	return nil
}

func (m *HAMAMapper) ensureLoaded(ctx context.Context) error {
	m.mu.RLock()
	fresh := !m.loadedAt.IsZero() && time.Since(m.loadedAt) < 24*time.Hour && len(m.byAniList)+len(m.byMAL) > 0
	m.mu.RUnlock()
	if fresh {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/Fribb/anime-lists/master/anime-list-full.json", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StormFlix-HAMA-Bridge/1.0")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("HAMA mapping source HTTP " + strconv.Itoa(resp.StatusCode))
	}
	var entries []hamaMapEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return err
	}
	byAniList := map[int64]hamaMapEntry{}
	byMAL := map[int64]hamaMapEntry{}
	byAniDB := map[int64]hamaMapEntry{}
	for _, entry := range entries {
		if entry.AniListID > 0 {
			byAniList[entry.AniListID] = entry
		}
		if entry.MALID > 0 {
			byMAL[entry.MALID] = entry
		}
		if entry.AniDBID > 0 {
			byAniDB[entry.AniDBID] = entry
		}
	}
	if len(byAniList)+len(byMAL)+len(byAniDB) == 0 {
		return errors.New("HAMA mapping source was empty")
	}
	m.mu.Lock()
	m.byAniList = byAniList
	m.byMAL = byMAL
	m.byAniDB = byAniDB
	m.loadedAt = time.Now()
	m.mu.Unlock()
	return nil
}

func hamaProviderLabel(result Result) string {
	parts := []string{}
	if result.AniListID > 0 {
		parts = append(parts, "AniList:"+strconv.FormatInt(result.AniListID, 10))
	}
	if result.MALID > 0 {
		parts = append(parts, "MAL:"+strconv.FormatInt(result.MALID, 10))
	}
	if result.TVDBID > 0 {
		parts = append(parts, "TVDB:"+strconv.FormatInt(result.TVDBID, 10))
	}
	return strings.Join(parts, " · ")
}
