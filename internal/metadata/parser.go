package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	yearRE          = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	seasonRE        = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]*E(\d{1,3})\b`)
	xEpisode        = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3})\b`)
	serialEpisodeRE = regexp.MustCompile(`(?i)(?:^|[ ._-])(?:ep(?:isode|isodio|isódio)?[ ._-]*)?(\d{1,3})$`)
	bracketRE       = regexp.MustCompile(`\[[^\]]+\]|\([^\)]*(?:1080|2160|720|480|x26|hevc|av1|web|bluray|remux|hdr|dv)[^\)]*\)`)
	junkParenRE     = regexp.MustCompile(`(?i)\((?:vhs|dvd|bdrip|bluray|blu-ray|dublado|dual[ ._-]*audio|legendado|webrip|web-dl|remux)\)`)
	movieIndexRE    = regexp.MustCompile(`(?i)\b(?:filme|movie)\s*[-_. ]*\d{1,3}\b`)
	seasonDirRE     = regexp.MustCompile(`(?i)^(?:season|temporada)\s*\d{1,3}$`)
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

func ParseFilename(path, libraryKind string) ParsedName {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	clean := cleanMetadataText(base)

	var out ParsedName
	animeCapable := libraryKind == "anime" || libraryKind == "mixed"
	out.LikelyMovie = libraryKind == "movies" || (animeCapable && animeMoviePath(path))
	simpleEpisode := false
	if match := seasonRE.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
		out.LikelyMovie = false
	} else if match := xEpisode.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
		out.LikelyMovie = false
	} else if libraryKind == "series" || (animeCapable && !out.LikelyMovie) {
		if match := serialEpisodeRE.FindStringSubmatch(clean); len(match) == 2 && !movieIndexRE.MatchString(clean) {
			episode, _ := strconv.Atoi(match[1])
			if episode > 0 {
				prefix := compactTitle(strings.TrimSuffix(clean, match[0]))
				if prefix != "" || meaningfulAncestor(path, clean) != "" {
					out.Season = 1
					out.Episode = episode
					clean = prefix
					out.LikelyMovie = false
					simpleEpisode = true
				}
			}
		}
	}

	if match := yearRE.FindStringSubmatch(clean); len(match) == 2 {
		out.Year, _ = strconv.Atoi(match[1])
		clean = strings.Replace(clean, match[0], " ", 1)
	}

	clean = trimReleaseTail(clean)
	originalTitle := compactTitle(clean)
	out.Title = originalTitle

	// Number-only episode naming such as "Dragon Quest - 01" is common in
	// anime releases. The parent folder is a much stronger series identity.
	if simpleEpisode {
		if parent := meaningfulAncestor(path, originalTitle); parent != "" {
			if originalTitle != "" {
				out.Alternates = append(out.Alternates, originalTitle)
			}
			out.Title = parent
		}
	}

	// Anime collections commonly use names such as "Filme 15 - O Renascimento
	// de Freeza". The movie number is useful for humans but usually hurts
	// provider search, so remove it and use the franchise directory as context.
	if animeCapable && movieIndexRE.MatchString(out.Title) {
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

	// Browser/download generated filenames such as "videoplayback.mp4" carry no
	// identity. In that case the parent directory is normally the actual title.
	if isGenericMediaName(out.Title) {
		if parent := meaningfulAncestor(path, originalTitle, out.Title); parent != "" {
			out.Alternates = append(out.Alternates, out.Title)
			out.Title = parent
		}
	}

	// Episode files are often named only "S01E01" while the series title is
	// carried by the parent directory.
	if (libraryKind == "series" || animeCapable) && len(strings.TrimSpace(out.Title)) < 2 {
		if parent := meaningfulAncestor(path, originalTitle, out.Title); parent != "" {
			out.Title = parent
		}
	}

	out.Title = compactTitle(out.Title)
	out.Alternates = uniqueTitles(out.Title, out.Alternates)
	return out
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
	}
	for _, token := range known {
		if w == token {
			return true
		}
	}
	return strings.HasPrefix(w, "1080") || strings.HasPrefix(w, "2160") || strings.HasPrefix(w, "720")
}
