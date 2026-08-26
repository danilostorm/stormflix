package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
)

func kidsGenresAllowed(genres []string) bool {
	for _, genre := range genres {
		g := strings.ToLower(strings.TrimSpace(genre))
		switch g {
		case "family", "família", "familia", "kids", "kid", "children", "children & family", "infantil", "crianças", "criancas":
			return true
		}
	}
	return false
}

type profileRestriction struct {
	Restricted bool
	Kids       bool
	Limit      int
}

func (s *server) selectedProfileRestriction(r *http.Request, userID int64) profileRestriction {
	profileID := s.selectedProfileID(r, userID)
	if profileID <= 0 {
		return profileRestriction{Limit: 18}
	}
	var kids bool
	limit := 18
	if err := s.db.QueryRowContext(r.Context(), `SELECT is_kids,content_rating_limit FROM profiles WHERE id=? AND user_id=? AND active=1`, profileID, userID).Scan(&kids, &limit); err != nil {
		return profileRestriction{Limit: 18}
	}
	if limit < 0 || limit > 18 {
		limit = 18
	}
	return profileRestriction{Restricted: kids || limit < 18, Kids: kids, Limit: limit}
}

func (s *server) selectedProfileIsKids(r *http.Request, userID int64) bool {
	return s.selectedProfileRestriction(r, userID).Restricted
}

func (s *server) ratingAges(ctx context.Context, mediaIDs []int64) (map[int64]int, error) {
	out := map[int64]int{}
	if len(mediaIDs) == 0 {
		return out, nil
	}
	seen := map[int64]bool{}
	marks := make([]string, 0, len(mediaIDs))
	args := make([]any, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		marks = append(marks, "?")
		args = append(args, id)
	}
	if len(marks) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT media_id,COALESCE(content_rating_age,-1) FROM media_metadata WHERE media_id IN (`+strings.Join(marks, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var age int
		if err := rows.Scan(&id, &age); err != nil {
			return nil, err
		}
		out[id] = age
	}
	return out, rows.Err()
}

func ratingAllowed(age, limit int, genres []string) bool {
	if age >= 0 {
		return age <= limit
	}
	// Unknown classification is never guessed safe for strict Livre profiles.
	// For other restricted profiles Family/Kids metadata is the conservative
	// fallback until the provider supplies a real classification.
	return limit >= 10 && kidsGenresAllowed(genres)
}

func (s *server) filterRestrictedItems(r *http.Request, userID int64, items []media.Item) []media.Item {
	restriction := s.selectedProfileRestriction(r, userID)
	if !restriction.Restricted || len(items) == 0 {
		return items
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	ages, err := s.ratingAges(r.Context(), ids)
	if err != nil {
		return []media.Item{}
	}
	out := make([]media.Item, 0, len(items))
	for _, item := range items {
		age, ok := ages[item.ID]
		if !ok {
			age = -1
		}
		if ratingAllowed(age, restriction.Limit, item.Genres) {
			out = append(out, item)
		}
	}
	return out
}

func (s *server) filterRestrictedSeries(r *http.Request, userID int64, items []media.SeriesSummary) []media.SeriesSummary {
	restriction := s.selectedProfileRestriction(r, userID)
	if !restriction.Restricted || len(items) == 0 {
		return items
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RepresentativeMediaID)
	}
	ages, err := s.ratingAges(r.Context(), ids)
	if err != nil {
		return []media.SeriesSummary{}
	}
	out := make([]media.SeriesSummary, 0, len(items))
	for _, item := range items {
		age, ok := ages[item.RepresentativeMediaID]
		if !ok {
			age = -1
		}
		if ratingAllowed(age, restriction.Limit, item.Genres) {
			out = append(out, item)
		}
	}
	return out
}

func (s *server) filterRestrictedHome(r *http.Request, userID int64, feed *media.HomeFeed) {
	if feed == nil || !s.selectedProfileRestriction(r, userID).Restricted {
		return
	}
	rows := make([]media.HomeRow, 0, len(feed.Rows))
	for _, row := range feed.Rows {
		row.Items = s.filterRestrictedItems(r, userID, row.Items)
		if len(row.Items) > 0 {
			rows = append(rows, row)
		}
	}
	feed.Rows = rows
	if feed.Hero != nil {
		allowed := s.filterRestrictedItems(r, userID, []media.Item{*feed.Hero})
		if len(allowed) == 0 {
			feed.Hero = nil
			for _, row := range feed.Rows {
				if len(row.Items) > 0 {
					candidate := row.Items[0]
					feed.Hero = &candidate
					break
				}
			}
		}
	}
}

func (s *server) mediaAllowedForKids(ctx context.Context, mediaID int64, limit int) bool {
	var genresJSON string
	var age int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(mm.genres_json,'[]'),COALESCE(mm.content_rating_age,-1) FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=? AND m.available=1`, mediaID).Scan(&genresJSON, &age); err != nil {
		return false
	}
	var genres []string
	if json.Unmarshal([]byte(genresJSON), &genres) != nil {
		return false
	}
	return ratingAllowed(age, limit, genres)
}

func (s *server) requireKidsMediaAccess(w http.ResponseWriter, r *http.Request, userID, mediaID int64) bool {
	restriction := s.selectedProfileRestriction(r, userID)
	if !restriction.Restricted {
		return true
	}
	if s.mediaAllowedForKids(r.Context(), mediaID, restriction.Limit) {
		return true
	}
	writeError(w, http.StatusForbidden, errKidsRestricted)
	return false
}

var errKidsRestricted = &kidsRestrictedError{}

type kidsRestrictedError struct{}

func (*kidsRestrictedError) Error() string { return "conteúdo indisponível para a classificação deste perfil" }
