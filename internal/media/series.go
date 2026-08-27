package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	seriesSeasonEpisodeRE = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]*E(\d{1,3})\b`)
	seriesXEpisodeRE      = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3})\b`)
	seriesSeasonDirRE     = regexp.MustCompile(`(?i)^(?:season|temporada)[ ._-]*(\d{1,3})$`)
	seriesTitleSuffixRE   = regexp.MustCompile(`(?i)\s+S\d{1,2}E\d{1,3}(?:\s*[·:\-].*)?$`)
	seriesYearRE          = regexp.MustCompile(`\s*[\(\[]?(?:19\d{2}|20\d{2})[\)\]]?\s*$`)
)

type SeriesSummary struct {
	ID                    string   `json:"id"`
	EntityType            string   `json:"entity_type"`
	RepresentativeMediaID int64    `json:"representative_media_id"`
	LibraryID             int64    `json:"library_id"`
	LibraryName           string   `json:"library_name"`
	MediaType             string   `json:"media_type"`
	Title                 string   `json:"title"`
	Year                  int      `json:"year"`
	Overview              string   `json:"overview"`
	Genres                []string `json:"genres"`
	Rating                float64  `json:"rating"`
	PosterURL             string   `json:"poster_url"`
	BackdropURL           string   `json:"backdrop_url"`
	LogoURL               string   `json:"logo_url"`
	ModifiedUnix          int64    `json:"modified_unix"`
	SeasonCount           int      `json:"season_count"`
	EpisodeCount          int      `json:"episode_count"`
}

type SeriesSeason struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Episodes []Item `json:"episodes"`
}

type SeriesDetail struct {
	SeriesSummary
	Seasons []SeriesSeason `json:"seasons"`
}

type seriesEpisode struct {
	item        Item
	path        string
	libraryKind string
	seriesPath  string
	seriesName  string
}

func (s *Service) SeriesList(ctx context.Context, allowedLibraryIDs []int64, query string) ([]SeriesSummary, error) {
	groups, err := s.seriesGroups(ctx, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]SeriesSummary, 0, len(groups))
	for _, group := range groups {
		if query != "" && !strings.Contains(strings.ToLower(group.SeriesSummary.Title), query) {
			continue
		}
		out = append(out, group.SeriesSummary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ModifiedUnix == out[j].ModifiedUnix {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		}
		return out[i].ModifiedUnix > out[j].ModifiedUnix
	})
	return out, nil
}

func (s *Service) SeriesDetail(ctx context.Context, id string, allowedLibraryIDs []int64) (SeriesDetail, error) {
	groups, err := s.seriesGroups(ctx, allowedLibraryIDs)
	if err != nil {
		return SeriesDetail{}, err
	}
	for _, group := range groups {
		if group.ID == id {
			return group, nil
		}
	}
	return SeriesDetail{}, sql.ErrNoRows
}

func (s *Service) RecentEpisodes(ctx context.Context, allowedLibraryIDs []int64, limit int) ([]Item, error) {
	groups, err := s.seriesGroups(ctx, allowedLibraryIDs)
	if err != nil {
		return nil, err
	}
	items := []Item{}
	for _, group := range groups {
		for _, season := range group.Seasons {
			items = append(items, season.Episodes...)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ModifiedUnix > items[j].ModifiedUnix })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func isSeriesLikeLibraryKind(kind string) bool {
	return kind == "series" || kind == "anime_series"
}

func (s *Service) seriesGroups(ctx context.Context, allowedLibraryIDs []int64) ([]SeriesDetail, error) {
	args := []any{}
	where := `m.available=1 AND (l.kind='series' OR l.kind='anime_series' OR l.kind='anime' OR l.kind='mixed' OR COALESCE(mm.season_number,0)>0 OR COALESCE(mm.episode_number,0)>0)`
	if allowedLibraryIDs != nil {
		if len(allowedLibraryIDs) == 0 {
			return []SeriesDetail{}, nil
		}
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		where += ` AND m.library_id IN (` + strings.Join(marks, ",") + `)`
	}
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.library_id,l.name,l.kind,m.title,m.path,m.extension,m.size_bytes,m.modified_unix,m.available,
COALESCE(mm.media_type,''),COALESCE(mm.year,0),COALESCE(mm.season_number,0),COALESCE(mm.episode_number,0),COALESCE(mm.overview,''),COALESCE(mm.genres_json,'[]'),COALESCE(mm.rating,0),COALESCE(mm.runtime_minutes,0),COALESCE(mm.status,'pending'),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='poster' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='backdrop' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),''),
COALESCE((SELECT a.public_url FROM media_artwork a WHERE a.media_id=m.id AND a.kind='logo' AND a.selected=1 ORDER BY a.score DESC LIMIT 1),'')
FROM media m JOIN libraries l ON l.id=m.library_id LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE `+where+` ORDER BY m.library_id,m.path`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byKey := map[string]*SeriesDetail{}
	order := []string{}
	for rows.Next() {
		var item Item
		var libraryKind, path, genresJSON string
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.LibraryName, &libraryKind, &item.Title, &path, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available,
			&item.MediaType, &item.Year, &item.SeasonNumber, &item.EpisodeNumber, &item.Overview, &genresJSON, &item.Rating, &item.RuntimeMinutes, &item.MetadataStatus, &item.PosterURL, &item.BackdropURL, &item.LogoURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genresJSON), &item.Genres)
		season, episode := item.SeasonNumber, item.EpisodeNumber
		if season == 0 || episode == 0 {
			season, episode = episodeFromPath(path)
		}
		// Anime/mixed movie libraries must not become fake series just because
		// they live inside folders. Explicit series/anime-series libraries are
		// allowed to group poorly named episodes and assign deterministic order.
		if !isSeriesLikeLibraryKind(libraryKind) && episode == 0 {
			continue
		}
		if episode == 0 && isSeriesLikeLibraryKind(libraryKind) {
			season = maxInt(season, 1)
		}
		item.SeasonNumber, item.EpisodeNumber = season, episode
		item.MediaType = seriesMediaType(libraryKind, item.MediaType)
		seriesPath, seriesName := deriveSeriesIdentity(path)
		key := seriesKey(item.LibraryID, seriesPath)
		group := byKey[key]
		if group == nil {
			group = &SeriesDetail{SeriesSummary: SeriesSummary{
				ID: key, EntityType: "series", RepresentativeMediaID: item.ID, LibraryID: item.LibraryID, LibraryName: item.LibraryName,
				MediaType: item.MediaType, Title: seriesName, Year: item.Year, Overview: item.Overview, Genres: append([]string(nil), item.Genres...),
				Rating: item.Rating, PosterURL: item.PosterURL, BackdropURL: item.BackdropURL, LogoURL: item.LogoURL, ModifiedUnix: item.ModifiedUnix,
			}, Seasons: []SeriesSeason{}}
			byKey[key] = group
			order = append(order, key)
		}
		mergeSeriesMetadata(&group.SeriesSummary, item, seriesName)
		appendSeriesEpisode(group, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SeriesDetail, 0, len(order))
	for _, key := range order {
		group := byKey[key]
		for i := range group.Seasons {
			season := &group.Seasons[i]
			sort.SliceStable(season.Episodes, func(a, b int) bool {
				ea, eb := season.Episodes[a].EpisodeNumber, season.Episodes[b].EpisodeNumber
				if ea == 0 || eb == 0 || ea == eb {
					return season.Episodes[a].ModifiedUnix < season.Episodes[b].ModifiedUnix
				}
				return ea < eb
			})
			for j := range season.Episodes {
				if season.Episodes[j].EpisodeNumber == 0 {
					season.Episodes[j].EpisodeNumber = j + 1
				}
			}
		}
		sort.SliceStable(group.Seasons, func(i, j int) bool { return group.Seasons[i].Number < group.Seasons[j].Number })
		group.SeasonCount = len(group.Seasons)
		out = append(out, *group)
	}
	return out, nil
}

func appendSeriesEpisode(group *SeriesDetail, item Item) {
	seasonNumber := item.SeasonNumber
	if seasonNumber <= 0 {
		seasonNumber = 1
	}
	for i := range group.Seasons {
		if group.Seasons[i].Number == seasonNumber {
			group.Seasons[i].Episodes = append(group.Seasons[i].Episodes, item)
			group.EpisodeCount++
			return
		}
	}
	group.Seasons = append(group.Seasons, SeriesSeason{Number: seasonNumber, Title: fmt.Sprintf("Temporada %d", seasonNumber), Episodes: []Item{item}})
	group.EpisodeCount++
}

func mergeSeriesMetadata(summary *SeriesSummary, item Item, folderTitle string) {
	if item.ModifiedUnix > summary.ModifiedUnix {
		summary.ModifiedUnix = item.ModifiedUnix
	}
	matchedTitle := strings.TrimSpace(seriesTitleSuffixRE.ReplaceAllString(item.Title, ""))
	if item.MetadataStatus == "matched" && matchedTitle != "" && !strings.EqualFold(matchedTitle, item.Title) {
		summary.Title = matchedTitle
	} else if summary.Title == "" {
		summary.Title = folderTitle
	}
	if summary.Year == 0 && item.Year > 0 {
		summary.Year = item.Year
	}
	if summary.Overview == "" && item.Overview != "" {
		summary.Overview = item.Overview
	}
	if summary.Rating == 0 && item.Rating > 0 {
		summary.Rating = item.Rating
	}
	if len(summary.Genres) == 0 && len(item.Genres) > 0 {
		summary.Genres = append([]string(nil), item.Genres...)
	}
	if summary.PosterURL == "" && item.PosterURL != "" {
		summary.PosterURL = item.PosterURL
		summary.RepresentativeMediaID = item.ID
	}
	if summary.BackdropURL == "" && item.BackdropURL != "" {
		summary.BackdropURL = item.BackdropURL
	}
	if summary.LogoURL == "" && item.LogoURL != "" {
		summary.LogoURL = item.LogoURL
	}
}

func deriveSeriesIdentity(path string) (string, string) {
	dir := filepath.Dir(path)
	base := filepath.Base(dir)
	if seriesSeasonDirRE.MatchString(base) {
		dir = filepath.Dir(dir)
		base = filepath.Base(dir)
	}
	name := strings.NewReplacer(".", " ", "_", " ").Replace(base)
	name = seriesYearRE.ReplaceAllString(name, "")
	name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
	if name == "" {
		name = "Série"
	}
	return filepath.Clean(dir), name
}

func episodeFromPath(path string) (int, int) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if m := seriesSeasonEpisodeRE.FindStringSubmatch(name); len(m) == 3 {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return s, e
	}
	if m := seriesXEpisodeRE.FindStringSubmatch(name); len(m) == 3 {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return s, e
	}
	if m := seriesSeasonDirRE.FindStringSubmatch(filepath.Base(filepath.Dir(path))); len(m) == 2 {
		s, _ := strconv.Atoi(m[1])
		return s, 0
	}
	return 0, 0
}

func seriesKey(libraryID int64, path string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(path))))
	return fmt.Sprintf("lib%d-%x", libraryID, h.Sum64())
}

func seriesMediaType(libraryKind, metadataType string) string {
	if libraryKind == "anime" || libraryKind == "anime_series" || strings.EqualFold(metadataType, "anime") {
		return "anime"
	}
	return "series"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
