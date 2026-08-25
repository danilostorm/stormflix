package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	yearRE    = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	seasonRE  = regexp.MustCompile(`(?i)\bS(\d{1,2})[ ._-]*E(\d{1,3})\b`)
	xEpisode  = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{1,3})\b`)
	bracketRE = regexp.MustCompile(`\[[^\]]+\]|\([^\)]*(?:1080|2160|720|480|x26|hevc|av1|web|bluray|remux|hdr|dv)[^\)]*\)`)
)

type ParsedName struct {
	Title   string
	Year    int
	Season  int
	Episode int
}

func ParseFilename(path, libraryKind string) ParsedName {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = bracketRE.ReplaceAllString(base, " ")
	clean := strings.NewReplacer(".", " ", "_", " ").Replace(base)

	var out ParsedName
	if match := seasonRE.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
	} else if match := xEpisode.FindStringSubmatch(clean); len(match) == 3 {
		out.Season, _ = strconv.Atoi(match[1])
		out.Episode, _ = strconv.Atoi(match[2])
		clean = strings.Replace(clean, match[0], " ", 1)
	}

	if match := yearRE.FindStringSubmatch(clean); len(match) == 2 {
		out.Year, _ = strconv.Atoi(match[1])
		clean = strings.Replace(clean, match[0], " ", 1)
	}

	words := strings.Fields(clean)
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if isReleaseToken(word) {
			break
		}
		kept = append(kept, word)
	}
	out.Title = strings.TrimSpace(strings.Join(kept, " "))
	if out.Title == "" {
		out.Title = strings.TrimSpace(strings.Join(strings.Fields(clean), " "))
	}

	// Episode files are often named only "S01E01" while the series title is
	// carried by the parent directory. Fall back to the first meaningful
	// parent directory for series/anime libraries.
	if (libraryKind == "series" || libraryKind == "anime") && len(out.Title) < 2 {
		dir := filepath.Dir(path)
		parent := filepath.Base(dir)
		if strings.HasPrefix(strings.ToLower(parent), "season ") || strings.HasPrefix(strings.ToLower(parent), "temporada ") {
			parent = filepath.Base(filepath.Dir(dir))
		}
		out.Title = strings.TrimSpace(strings.NewReplacer(".", " ", "_", " ").Replace(parent))
	}
	return out
}

func isReleaseToken(word string) bool {
	w := strings.ToLower(strings.Trim(word, "-[](){}"))
	if w == "" {
		return false
	}
	known := []string{
		"2160p", "1080p", "720p", "480p", "4k", "uhd", "hdr", "hdr10", "dv", "dolbyvision",
		"bluray", "bdrip", "brrip", "web", "web-dl", "webdl", "webrip", "hdtv", "remux",
		"x264", "x265", "h264", "h265", "hevc", "av1", "xvid", "aac", "ac3", "eac3", "dts", "truehd",
	}
	for _, token := range known {
		if w == token {
			return true
		}
	}
	return strings.HasPrefix(w, "1080") || strings.HasPrefix(w, "2160") || strings.HasPrefix(w, "720")
}
