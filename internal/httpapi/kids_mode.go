package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/danilostorm/stormflix/internal/media"
)

// Kids mode is intentionally conservative. A title is visible only when its
// provider metadata explicitly marks it as Family/Kids. Unknown/unmatched
// titles are hidden instead of being guessed safe from filename or media type.
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

func filterKidsItems(items []media.Item) []media.Item {
	out := make([]media.Item, 0, len(items))
	for _, item := range items {
		if kidsGenresAllowed(item.Genres) {
			out = append(out, item)
		}
	}
	return out
}

func filterKidsSeries(items []media.SeriesSummary) []media.SeriesSummary {
	out := make([]media.SeriesSummary, 0, len(items))
	for _, item := range items {
		if kidsGenresAllowed(item.Genres) {
			out = append(out, item)
		}
	}
	return out
}

func filterKidsHome(feed *media.HomeFeed) {
	if feed == nil {
		return
	}
	rows := make([]media.HomeRow, 0, len(feed.Rows))
	for _, row := range feed.Rows {
		row.Items = filterKidsItems(row.Items)
		if len(row.Items) > 0 {
			rows = append(rows, row)
		}
	}
	feed.Rows = rows
	if feed.Hero != nil && !kidsGenresAllowed(feed.Hero.Genres) {
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

func (s *server) selectedProfileIsKids(r *http.Request, userID int64) bool {
	profileID := s.selectedProfileID(r, userID)
	if profileID <= 0 {
		return false
	}
	var kids bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT is_kids FROM profiles WHERE id=? AND user_id=? AND active=1`, profileID, userID).Scan(&kids); err != nil {
		return false
	}
	return kids
}

func (s *server) mediaAllowedForKids(ctx context.Context, mediaID int64) bool {
	var genresJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(mm.genres_json,'[]') FROM media m LEFT JOIN media_metadata mm ON mm.media_id=m.id WHERE m.id=? AND m.available=1`, mediaID).Scan(&genresJSON); err != nil {
		return false
	}
	var genres []string
	if json.Unmarshal([]byte(genresJSON), &genres) != nil {
		return false
	}
	return kidsGenresAllowed(genres)
}

func (s *server) requireKidsMediaAccess(w http.ResponseWriter, r *http.Request, userID, mediaID int64) bool {
	if !s.selectedProfileIsKids(r, userID) {
		return true
	}
	if s.mediaAllowedForKids(r.Context(), mediaID) {
		return true
	}
	writeError(w, http.StatusForbidden, errKidsRestricted)
	return false
}

var errKidsRestricted = &kidsRestrictedError{}

type kidsRestrictedError struct{}

func (*kidsRestrictedError) Error() string { return "conteúdo indisponível neste perfil infantil" }
