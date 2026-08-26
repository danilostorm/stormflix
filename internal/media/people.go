package media

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

type PersonFilmography struct {
	Person Person `json:"person"`
	Items  []Item `json:"items"`
}

// PersonTitles returns only titles already present in StormFlix where the
// requested person exists in the provider cast data. Matching is exact after
// trimming/case-folding, so similar actor names do not bleed into each other.
func (s *Service) PersonTitles(ctx context.Context, name string, allowedLibraryIDs []int64) (PersonFilmography, error) {
	name = strings.TrimSpace(name)
	result := PersonFilmography{Person: Person{Name: name}, Items: []Item{}}
	if name == "" {
		return result, nil
	}

	rows, err := s.db.QueryContext(ctx, `SELECT media_id,cast_json FROM media_metadata WHERE cast_json LIKE ?`, "%"+name+"%")
	if err != nil {
		return result, err
	}
	matched := map[int64]bool{}
	for rows.Next() {
		var mediaID int64
		var castJSON string
		if err := rows.Scan(&mediaID, &castJSON); err != nil {
			_ = rows.Close()
			return result, err
		}
		var cast []Person
		if json.Unmarshal([]byte(castJSON), &cast) != nil {
			continue
		}
		for _, person := range cast {
			if strings.EqualFold(strings.TrimSpace(person.Name), name) {
				matched[mediaID] = true
				if result.Person.ProfileURL == "" && person.ProfileURL != "" {
					result.Person = person
				}
				break
			}
		}
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	if len(matched) == 0 {
		return result, nil
	}

	for offset := 0; ; offset += 500 {
		items, err := s.List(ctx, 0, "", 500, offset, allowedLibraryIDs)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if matched[item.ID] {
				result.Items = append(result.Items, item)
			}
		}
		if len(items) < 500 {
			break
		}
	}
	result.Items = DedupeItems(result.Items)
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].Year == result.Items[j].Year {
			return result.Items[i].Title < result.Items[j].Title
		}
		return result.Items[i].Year > result.Items[j].Year
	})
	return result, nil
}
