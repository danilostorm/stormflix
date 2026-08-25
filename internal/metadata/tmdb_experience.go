package metadata

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type tmdbExperience struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	Tagline       string `json:"tagline"`
	Credits       struct {
		Cast []struct {
			Name        string `json:"name"`
			Character   string `json:"character"`
			ProfilePath string `json:"profile_path"`
			Order       int    `json:"order"`
		} `json:"cast"`
		Crew []struct {
			Name       string `json:"name"`
			Job        string `json:"job"`
			Department string `json:"department"`
		} `json:"crew"`
	} `json:"credits"`
	Videos struct {
		Results []struct {
			Key      string `json:"key"`
			Site     string `json:"site"`
			Type     string `json:"type"`
			Official bool   `json:"official"`
			Name     string `json:"name"`
		} `json:"results"`
	} `json:"videos"`
}

func (p *TMDBProvider) EnrichExperience(ctx context.Context, result *Result) error {
	if result == nil || result.TMDBID <= 0 || !p.Ready() {
		return nil
	}
	kind := "tv"
	if result.MediaType == "movie" {
		kind = "movie"
	}
	q := url.Values{}
	if p.language != "" {
		q.Set("language", p.language)
	}
	q.Set("append_to_response", "credits,videos")
	var data tmdbExperience
	if err := p.get(ctx, fmt.Sprintf("https://api.themoviedb.org/3/%s/%d?%s", kind, result.TMDBID, q.Encode()), &data); err != nil {
		return err
	}
	if result.OriginalTitle == "" {
		result.OriginalTitle = firstNonEmpty(data.OriginalTitle, data.OriginalName)
	}
	if result.Tagline == "" {
		result.Tagline = data.Tagline
	}

	sort.SliceStable(data.Credits.Cast, func(i, j int) bool { return data.Credits.Cast[i].Order < data.Credits.Cast[j].Order })
	for _, cast := range data.Credits.Cast {
		if strings.TrimSpace(cast.Name) == "" {
			continue
		}
		profile := ""
		if cast.ProfilePath != "" {
			profile = "https://image.tmdb.org/t/p/w185" + cast.ProfilePath
		}
		result.Cast = append(result.Cast, Person{Name: cast.Name, Character: cast.Character, ProfileURL: profile})
		if len(result.Cast) >= 18 {
			break
		}
	}
	seenDirectors := map[string]bool{}
	for _, crew := range data.Credits.Crew {
		if !strings.EqualFold(crew.Job, "Director") && !strings.EqualFold(crew.Job, "Series Director") {
			continue
		}
		name := strings.TrimSpace(crew.Name)
		if name != "" && !seenDirectors[name] {
			seenDirectors[name] = true
			result.Directors = append(result.Directors, name)
		}
	}

	bestScore := -1
	bestKey := ""
	for _, video := range data.Videos.Results {
		if !strings.EqualFold(video.Site, "YouTube") || video.Key == "" {
			continue
		}
		score := 0
		if strings.EqualFold(video.Type, "Trailer") {
			score += 10
		}
		if video.Official {
			score += 5
		}
		if strings.Contains(strings.ToLower(video.Name), "official") {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			bestKey = video.Key
		}
	}
	if bestKey != "" {
		result.TrailerURL = "https://www.youtube.com/watch?v=" + bestKey
	}
	return nil
}
