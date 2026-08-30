package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type catalogHealthItem struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Library        string   `json:"library"`
	Available      bool     `json:"available"`
	MetadataStatus string   `json:"metadata_status"`
	Genres         []string `json:"genres"`
	PosterURL      string   `json:"poster_url"`
	TMDBID         int64    `json:"tmdb_id"`
	Year           int      `json:"year"`
	MediaType      string   `json:"media_type"`
	SeasonNumber   int      `json:"season_number,omitempty"`
	EpisodeNumber  int      `json:"episode_number,omitempty"`
}

type duplicateGroup struct {
	Key       string              `json:"key"`
	Title     string              `json:"title"`
	Year      int                 `json:"year"`
	MediaType string              `json:"media_type"`
	TMDBID    int64               `json:"tmdb_id"`
	Copies    []catalogHealthItem `json:"copies"`
}

func (s *server) allCatalogHealthItems(ctx context.Context) ([]catalogHealthItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.title,l.name,m.available,COALESCE(mm.status,'pending'),COALESCE(mm.genres_json,'[]'),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE(mm.tmdb_id,0),COALESCE(mm.year,0),COALESCE(mm.media_type,''),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0)
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE l.kind<>'music' ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalogHealthItem{}
	for rows.Next() {
		var item catalogHealthItem
		var genres string
		if err := rows.Scan(&item.ID, &item.Title, &item.Library, &item.Available, &item.MetadataStatus, &genres, &item.PosterURL, &item.TMDBID, &item.Year, &item.MediaType, &item.SeasonNumber, &item.EpisodeNumber); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &item.Genres)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *server) catalogHealth(w http.ResponseWriter, r *http.Request) {
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	metrics := map[string]int{"total": 0, "sem_metadados": 0, "sem_capa": 0, "sem_genero": 0, "outros": 0, "indisponiveis": 0, "duplicados": 0, "tecnico_pendente": 0}
	for _, item := range items {
		if item.Available {
			metrics["total"]++
		}
		if item.Available && (item.MetadataStatus == "" || item.MetadataStatus == "pending" || item.MetadataStatus == "error") {
			metrics["sem_metadados"]++
		}
		if item.Available && strings.TrimSpace(item.PosterURL) == "" {
			metrics["sem_capa"]++
		}
		if item.Available && len(item.Genres) == 0 {
			metrics["sem_genero"]++
		}
		if item.Available && catalogItemFallsInOutros(item.Genres) {
			metrics["outros"]++
		}
		if !item.Available {
			metrics["indisponiveis"]++
		}
	}
	metrics["duplicados"] = len(duplicateCatalogGroups(items))
	var technicalPending int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_technical mt ON mt.media_id=m.id WHERE m.available=1 AND l.kind<>'music' AND (mt.media_id IS NULL OR mt.status<>'ok' OR mt.source_modified_unix<>m.modified_unix)`).Scan(&technicalPending)
	metrics["tecnico_pendente"] = technicalPending
	writeJSON(w, http.StatusOK, metrics)
}

func catalogItemFallsInOutros(genres []string) bool {
	if len(genres) == 0 {
		return true
	}
	known := map[string]bool{
		"action": true, "acao": true, "adventure": true, "aventura": true, "animation": true, "animacao": true, "comedy": true, "comedia": true,
		"crime": true, "documentary": true, "documentario": true, "documentarios": true, "drama": true, "family": true, "familia": true,
		"fantasy": true, "fantasia": true, "history": true, "historia": true, "horror": true, "terror": true, "music": true, "musica": true,
		"mystery": true, "misterio": true, "romance": true, "science-fiction": true, "sci-fi": true, "ficcao-cientifica": true,
		"thriller": true, "suspense": true, "war": true, "guerra": true, "western": true, "faroeste": true, "kids": true, "infantil": true,
		"reality": true, "reality-show": true, "soap": true, "novela": true, "novelas": true, "news": true, "noticias": true,
		"sports": true, "esportes": true, "supernatural": true, "sobrenatural": true,
	}
	for _, genre := range genres {
		if known[categoryRuleKey(genre)] {
			return false
		}
	}
	return true
}

func (s *server) catalogHealthItems(w http.ResponseWriter, r *http.Request) {
	issue := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("issue")))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	filtered := []catalogHealthItem{}
	for _, item := range items {
		match := false
		switch issue {
		case "sem_metadados":
			match = item.Available && (item.MetadataStatus == "" || item.MetadataStatus == "pending" || item.MetadataStatus == "error")
		case "sem_capa":
			match = item.Available && item.PosterURL == ""
		case "sem_genero":
			match = item.Available && len(item.Genres) == 0
		case "outros":
			match = item.Available && catalogItemFallsInOutros(item.Genres)
		case "indisponiveis":
			match = !item.Available
		default:
			match = item.Available
		}
		if match {
			filtered = append(filtered, item)
		}
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": total, "items": filtered[offset:end]})
}

func duplicateCatalogGroups(items []catalogHealthItem) []duplicateGroup {
	groups := map[string][]catalogHealthItem{}
	for _, item := range items {
		if !item.Available {
			continue
		}
		key := ""
		if item.TMDBID > 0 {
			key = fmt.Sprintf("tmdb:%d:%s:s%d:e%d", item.TMDBID, strings.ToLower(item.MediaType), item.SeasonNumber, item.EpisodeNumber)
		} else {
			key = fmt.Sprintf("title:%s:%d:%s:s%d:e%d", categoryRuleKey(item.Title), item.Year, strings.ToLower(item.MediaType), item.SeasonNumber, item.EpisodeNumber)
		}
		groups[key] = append(groups[key], item)
	}
	out := []duplicateGroup{}
	for key, copies := range groups {
		if len(copies) < 2 {
			continue
		}
		first := copies[0]
		out = append(out, duplicateGroup{Key: key, Title: first.Title, Year: first.Year, MediaType: first.MediaType, TMDBID: first.TMDBID, Copies: copies})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Copies) == len(out[j].Copies) {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return len(out[i].Copies) > len(out[j].Copies)
	})
	return out
}

func (s *server) catalogDuplicates(w http.ResponseWriter, r *http.Request) {
	items, err := s.allCatalogHealthItems(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	groups := duplicateCatalogGroups(items)
	if len(groups) > 100 {
		groups = groups[:100]
	}
	writeJSON(w, http.StatusOK, groups)
}
