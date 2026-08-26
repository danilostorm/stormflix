package metadata

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type tmdbExperience struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	Tagline       string `json:"tagline"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
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
	ReleaseDates struct {
		Results []struct {
			ISO31661    string `json:"iso_3166_1"`
			ReleaseDates []struct {
				Certification string `json:"certification"`
				Type          int    `json:"type"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Results []struct {
			ISO31661 string `json:"iso_3166_1"`
			Rating   string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

func (p *TMDBProvider) EnrichExperience(ctx context.Context, result *Result) error {
	if result == nil || result.TMDBID <= 0 || !p.Ready() {
		return nil
	}
	kind := "tv"
	appendFields := "credits,videos,content_ratings"
	if result.MediaType == "movie" {
		kind = "movie"
		appendFields = "credits,videos,release_dates"
	}
	q := url.Values{}
	if p.language != "" {
		q.Set("language", p.language)
	}
	q.Set("append_to_response", appendFields)
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
	if result.ReleaseDate == "" {
		result.ReleaseDate = firstNonEmpty(data.ReleaseDate, data.FirstAirDate)
	}
	result.ContentRating = ""
	result.ContentRatingAge = -1
	if kind == "movie" {
		result.ContentRating = bestMovieCertification(data)
	} else {
		result.ContentRating = bestTVCertification(data)
	}
	if result.ContentRating != "" {
		result.ContentRatingAge = certificationAge(result.ContentRating)
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

func bestMovieCertification(data tmdbExperience) string {
	for _, country := range []string{"BR", "US"} {
		for _, group := range data.ReleaseDates.Results {
			if !strings.EqualFold(group.ISO31661, country) {
				continue
			}
			best := ""
			bestType := 99
			for _, release := range group.ReleaseDates {
				cert := strings.TrimSpace(release.Certification)
				if cert == "" {
					continue
				}
				if release.Type < bestType {
					best = cert
					bestType = release.Type
				}
			}
			if best != "" {
				return best
			}
		}
	}
	return ""
}

func bestTVCertification(data tmdbExperience) string {
	for _, country := range []string{"BR", "US"} {
		for _, rating := range data.ContentRatings.Results {
			if strings.EqualFold(rating.ISO31661, country) && strings.TrimSpace(rating.Rating) != "" {
				return strings.TrimSpace(rating.Rating)
			}
		}
	}
	return ""
}

// certificationAge normalizes Brazilian classifications first and maps common
// US TV/MPAA values when TMDB has no Brazilian entry. -1 means unknown.
func certificationAge(value string) int {
	v := strings.ToUpper(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, " ", "")
	switch v {
	case "L", "LIVRE", "G", "TV-G", "TV-Y", "TV-Y7":
		return 0
	case "10":
		return 10
	case "12", "PG", "TV-PG":
		return 12
	case "14", "PG-13", "TV-14":
		return 14
	case "16", "R":
		return 16
	case "18", "NC-17", "TV-MA":
		return 18
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 21 {
		return n
	}
	return -1
}
