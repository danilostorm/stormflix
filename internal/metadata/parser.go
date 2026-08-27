package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	yearRE                    = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	twoDigitYearRE            = regexp.MustCompile(`(?:^|[ ._-])(\d{2})(?:$|[ ._-])`)
	seasonRE                  = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]*E[ ._-]*(\d{1,3})\b`)
	xEpisode                  = regexp.MustCompile(`(?i)\b(\d{1,2})x[ ._-]*(\d{1,3})\b`)
	serialEpisodeRE           = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:ep(?:isode|isodio|isódio)?[ ._-]*)?(\d{1,3})$`)
	leadingEpisodeRE          = regexp.MustCompile(`(?i)^(?:ep(?:isode|isodio|isódio)?[ ._-]*)?(\d{1,3})(?:[ ._-]+|$)`)
	animeEmbeddedEpisodeRE    = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:ep(?:i|isode|isodio|isódio)?)[ ._-]*(\d{1,3})`)
	animeStandaloneEpisodeRE  = regexp.MustCompile(`(?:^|[ ._-])(\d{1,3})(?:[ ._-]|$)`)
	bracketRE                 = regexp.MustCompile(`\[[^\]]+\]|\([^\)]*(?:1080|2160|720|480|x26|hevc|av1|web|bluray|remux|hdr|dv)[^\)]*\)`)
	junkParenRE               = regexp.MustCompile(`(?i)\((?:vhs|dvd|bdrip|bluray|blu-ray|dublado|dual[ ._-]*audio|legendado|webrip|web-dl|remux)\)`)
	emptyGroupRE              = regexp.MustCompile(`\(\s*\)|\[\s*\]|\{\s*\}`)
	movieIndexRE              = regexp.MustCompile(`(?i)\b(?:filme|movie)\s*[-_. ]*\d{1,3}\b`)
	seasonDirRE               = regexp.MustCompile(`(?i)^(?:season|temporada)\s*\d{1,3}$`)
	seasonDirNumberRE         = regexp.MustCompile(`(?i)^(?:season|temporada)\s*(\d{1,3})$`)
	wordNumberEndRE           = regexp.MustCompile(`(?i)([[:alpha:]])(\d{1,2})(?:$|[ ._-])`)
)

type ParsedName struct {
	Title       string
	Year        int
	Season      int
	Episode     int
	LikelyMovie bool
	Alternates  []string
}

func (p ParsedName) SearchTitles() []string {
	out := make([]string, 0, 1+len(p.Alternates))
	seen := map[string]bool{}
	for _, value := range append([]string{p.Title}, p.Alternates...) {
		value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
		key := normalizeTitle(value)
		if value == "" || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func isAnimeCapableLibraryKind(kind string) bool {
	return kind == "anime" || kind == "mixed" || kind == "anime_series"
}

func isSeriesLibraryKind(kind string) bool {
	return kind == "series" || kind == "anime_series"
}

func ParseFilename(path, libraryKind string) ParsedName {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	clean := cleanMetadataText(base)

	var out ParsedName
	animeCapable := isAnimeCapableLibraryKind(libraryKind)
	seriesLike := isSeriesLibraryKind(libraryKind)
	out.LikelyMovie = libraryKind == "movies" || (animeCapable && libraryKind != "anime_series" && animeMoviePath(path))
	simpleEpisode := false
	if match := seasonRE.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
		out.LikelyMovie = false
		simpleEpisode = libraryKind == "anime_series"
	} else if match := xEpisode.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
		out.LikelyMovie = false
		simpleEpisode = libraryKind == "anime_series"
	} else if libraryKind == "anime_series" {
		// Dubbed anime archives frequently carry the real show identity in the
		// parent folder and noisy release names in the file itself, for example:
		//   Inuyasha/InuYasha.107.MemoriadaTV.BDMenor.mkv
		//   Bucky/Bucky.18.Upscale1080p.MemoriadaTV.Maior.mkv
		//   Samurai X/SamuraiX-Epi90UpsAI1080p.MemoriadaTV.Maior.mkv
		// Extract only the episode number here; the parent directory becomes the
		// provider search title below.
		episode := 0
		if match := animeEmbeddedEpisodeRE.FindStringSubmatch(clean); len(match) == 2 {
			episode, _ = strconv.Atoi(match[1])
		} else if match := animeStandaloneEpisodeRE.FindStringSubmatch(clean); len(match) == 2 {
			episode, _ = strconv.Atoi(match[1])
		}
		if episode > 0 && episode <= 999 && meaningfulAncestor(path, clean) != "" {
			out.Season = seasonFromDirectory(path)
			if out.Season <= 0 {
				out.Season = 1
			}
			out.Episode = episode
			out.LikelyMovie = false
			clean = ""
			simpleEpisode = true
		}
	} else if seriesLike || (animeCapable && !out.LikelyMovie) {
		if match := serialEpisodeRE.FindStringSubmatch(clean); len(match) == 2 && !movieIndexRE.MatchString(clean) {
			episode, _ := strconv.Atoi(match[1])
			if episode > 0 {
				prefix := compactTitle(strings.TrimSuffix(clean, match[0]))
				if prefix != "" || meaningfulAncestor(path, clean) != "" {
					out.Season = seasonFromDirectory(path)
					if out.Season <= 0 {
						out.Season = 1
					}
					out.Episode = episode
					clean = prefix
					out.LikelyMovie = false
					simpleEpisode = true
				}
			}
		} else if match := leadingEpisodeRE.FindStringSubmatch(clean); len(match) == 2 {
			episode, _ := strconv.Atoi(match[1])
			if episode > 0 && episode <= 999 && meaningfulAncestor(path, clean) != "" {
				out.Season = seasonFromDirectory(path)
				if out.Season <= 0 {
					out.Season = 1
				}
				out.Episode = episode
				out.LikelyMovie = false
				clean = ""
				simpleEpisode = true
			}
		}
	}

	if match := yearRE.FindStringSubmatch(clean); len(match) == 2 {
		out.Year, _ = strconv.Atoi(match[1])
		clean = strings.Replace(clean, match[0], " ", 1)
	} else if (out.LikelyMovie || libraryKind == "movies" || libraryKind == "mixed") && !(animeCapable && movieIndexRE.MatchString(clean)) {
		if match := twoDigitYearRE.FindStringSubmatch(clean); len(match) == 2 {
			yy, _ := strconv.Atoi(match[1])
			if yy <= 29 {
				out.Year = 2000 + yy
			} else {
				out.Year = 1900 + yy
			}
			clean = strings.Replace(clean, match[0], " ", 1)
		}
	}
	clean = emptyGroupRE.ReplaceAllString(clean, " ")
	clean = trimReleaseTail(clean)
	originalTitle := compactTitle(clean)
	out.Title = originalTitle

	if simpleEpisode {
		if parent := meaningfulAncestor(path, originalTitle); parent != "" {
			if originalTitle != "" {
				out.Alternates = append(out.Alternates, originalTitle)
			}
			out.Title = parent
		}
	}

	if animeCapable && libraryKind != "anime_series" && movieIndexRE.MatchString(out.Title) {
		out.LikelyMovie = true
		withoutIndex := compactTitle(movieIndexRE.ReplaceAllString(out.Title, " "))
		withoutIndex = strings.Trim(withoutIndex, " -–—:·")
		withoutIndex = compactTitle(withoutIndex)
		if withoutIndex != "" {
			out.Title = withoutIndex
			out.Alternates = append(out.Alternates, originalTitle)
		}
		if franchise := meaningfulAncestor(path, originalTitle, out.Title); franchise != "" && !titleContains(out.Title, franchise) {
			out.Alternates = append(out.Alternates, out.Title)
			out.Title = compactTitle(franchise + " " + out.Title)
		}
	}

	if isGenericMediaName(out.Title) {
		if parent := meaningfulAncestor(path, originalTitle, out.Title); parent != "" {
			out.Alternates = append(out.Alternates, out.Title)
			out.Title = parent
		}
	}

	if (seriesLike || animeCapable) && len(strings.TrimSpace(out.Title)) < 2 {
		if parent := meaningfulAncestor(path, originalTitle, out.Title); parent != "" {
			out.Title = parent
		}
	}

	out.Title = compactTitle(out.Title)
	out.Alternates = uniqueTitles(out.Title, out.Alternates)
	return out
}

func seasonFromDirectory(path string) int {
	for depth, dir := 0, filepath.Dir(path); depth < 3; depth, dir = depth+1, filepath.Dir(dir) {
		name := cleanMetadataText(filepath.Base(dir))
		if match := seasonDirNumberRE.FindStringSubmatch(name); len(match) == 2 {
			n, _ := strconv.Atoi(match[1])
			return n
		}
		if next := filepath.Dir(dir); next == dir {
			break
		}
	}
	return 0
}

func animeMoviePath(path string) bool {
	clean := strings.ToLower(filepath.ToSlash(path))
	markers := []string{"/filmes animes/", "/filmes anime/", "/anime movies/", "/anime filmes/"}
	for _, marker := range markers {
		if strings.Contains(clean, marker) {
			return true
		}
	}
	return false
}

func cleanMetadataText(value string) string {
	value = bracketRE.ReplaceAllString(value, " ")
	value = junkParenRE.ReplaceAllString(value, " ")
	value = wordNumberEndRE.ReplaceAllString(value, `$1 $2 `)
	value = strings.ReplaceAll(value, "NAO", " Nao ")
	value = strings.NewReplacer(".", " ", "_", " ", "–", " - ", "—", " - ").Replace(value)
	return compactTitle(value)
}

func trimReleaseTail(value string) string {
	words := strings.Fields(value)
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if isReleaseToken(word) {
			break
		}
		kept = append(kept, word)
	}
	if len(kept) == 0 {
		return compactTitle(value)
	}
	return compactTitle(strings.Join(kept, " "))
}

func meaningfulAncestor(path string, ignored ...string) string {
	ignore := map[string]bool{}
	for _, value := range ignored {
		ignore[normalizeTitle(value)] = true
	}
	dir := filepath.Dir(path)
	for depth := 0; depth < 5; depth++ {
		name := cleanMetadataText(filepath.Base(dir))
		key := normalizeTitle(name)
		if name != "" && key != "" && !ignore[key] && !isGenericDirectory(name) {
			return name
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return ""
}

func isGenericDirectory(value string) bool {
	v := strings.ToLower(compactTitle(value))
	if seasonDirRE.MatchString(v) {
		return true
	}
	generic := map[string]bool{
		"filmes": true, "movies": true, "movie": true, "series": true, "séries": true,
		"anime": true, "animes": true, "filmes animes": true, "filmes anime": true,
		"animes dublado": true, "animes dublados": true, "anime dublado": true, "anime dublados": true,
		"dublado": true, "dublados": true, "legendado": true, "legendados": true,
		"media": true, "videos": true, "vídeos": true,
	}
	return generic[v]
}

func isGenericMediaName(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	generic := map[string]bool{
		"videoplayback": true, "video playback": true, "video": true, "movie": true,
		"filme": true, "output": true, "download": true, "arquivo": true,
	}
	return generic[v]
}

func titleContains(title, fragment string) bool {
	a, b := normalizeTitle(title), normalizeTitle(fragment)
	return a != "" && b != "" && strings.Contains(a, b)
}

func uniqueTitles(primary string, values []string) []string {
	seen := map[string]bool{normalizeTitle(primary): true}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = compactTitle(value)
		key := normalizeTitle(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func compactTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " -–—:·")
	return strings.Join(strings.Fields(value), " ")
}

func isReleaseToken(word string) bool {
	w := strings.ToLower(strings.Trim(word, "-[](){}"))
	if w == "" {
		return false
	}
	known := []string{
		"2160p", "1080p", "720p", "480p", "4k", "uhd", "hdr", "hdr10", "dv", "dolbyvision",
		"bluray", "blu-ray", "bdrip", "brrip", "web", "web-dl", "webdl", "webrip", "hdtv", "remux",
		"x264", "x265", "h264", "h265", "hevc", "av1", "xvid", "aac", "ac3", "eac3", "dts", "truehd",
		"10bit", "8bit", "atmos", "ddp5", "ddp", "multi", "dual", "dual-audio",
		"dublado", "legendado", "dc", "directorcut", "directorscut", "renegade", "trial", "medio",
		"memoriadatv", "bdmenor", "bdmaior", "maior", "menor",
	}
	for _, token := range known {
		if w == token {
			return true
		}
	}
	if strings.HasPrefix(w, "dual-") || strings.HasPrefix(w, "multi-") {
		return true
	}
	if strings.Contains(w, "2160p") || strings.Contains(w, "1080p") || strings.Contains(w, "720p") || strings.Contains(w, "480p") {
		return true
	}
	if strings.Contains(w, "bluray") || strings.Contains(w, "bdrip") || strings.Contains(w, "brrip") || strings.Contains(w, "remux") || strings.Contains(w, "webdl") || strings.Contains(w, "web-dl") {
		return true
	}
	return false
}
